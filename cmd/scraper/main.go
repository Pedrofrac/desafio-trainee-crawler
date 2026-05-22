package main

import (
	"crypto/sha256"
	"encoding/hex"
	_ "github.com/lib/pq"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gocolly/colly/v2"
)

type Book struct {
	Title        string  `json:"title"`
	Price        float64 `json:"price"`
	Rating       int     `json:"rating"`
	Availability string  `json:"availability"`
	ImageURL     string  `json:"image_url"`
}

var (
	ratingsMap = map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
		"four":  4,
		"five":  5,
	}
)

func cleanPrice(priceStr string) (float64, error) {
	trimmed := strings.TrimSpace(priceStr)
	if trimmed == "" {
		return 0.0, fmt.Errorf("string de preco vazia")
	}

	// Filtrar apenas dígitos, pontos e vírgulas
	var sb strings.Builder
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			sb.WriteRune(r)
		}
	}
	cleaned := sb.String()
	if cleaned == "" {
		return 0.0, fmt.Errorf("nenhum caractere numerico ou separador encontrado em %q", priceStr)
	}

	lastDot := strings.LastIndex(cleaned, ".")
	lastComma := strings.LastIndex(cleaned, ",")

	lastSepIdx := -1

	if lastDot != -1 && lastComma != -1 {
		// Caso possua ambos os separadores, o último na string é obrigatoriamente o decimal
		if lastDot > lastComma {
			lastSepIdx = lastDot
		} else {
			lastSepIdx = lastComma
		}
	} else if lastDot != -1 {
		// Possui apenas o ponto decimal. Se seguido de exatamente 3 dígitos, assume-se milhar.
		afterDot := len(cleaned) - 1 - lastDot
		if afterDot != 3 {
			lastSepIdx = lastDot
		}
	} else if lastComma != -1 {
		// Possui apenas a vírgula. Se seguida de exatamente 3 dígitos, assume-se milhar.
		afterComma := len(cleaned) - 1 - lastComma
		if afterComma != 3 {
			lastSepIdx = lastComma
		}
	}

	// Normalização para o padrão float do Go (apenas dígitos e opcionalmente um único ponto decimal)
	var finalSb strings.Builder
	for i, r := range cleaned {
		if r >= '0' && r <= '9' {
			finalSb.WriteRune(r)
		} else if i == lastSepIdx {
			finalSb.WriteRune('.')
		}
	}

	finalStr := finalSb.String()
	val, err := strconv.ParseFloat(finalStr, 64)
	if err != nil {
		return 0.0, fmt.Errorf("falha ao converter valor formatado %q em float64: %w", finalStr, err)
	}

	return val, nil
}

func mapRating(classStr string) int {
	parts := strings.Fields(strings.ToLower(classStr))
	for _, part := range parts {
		if val, ok := ratingsMap[part]; ok {
			return val
		}
	}
	slog.Warn("Nao foi possivel mapear a classe de rating. Valor retornado como 0.", "classe", classStr)
	return 0
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	migCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	const schema = `CREATE TABLE IF NOT EXISTS books (
		id SERIAL PRIMARY KEY, 
		title TEXT, 
		price NUMERIC, 
		rating INT, 
		availability TEXT, 
		image_url TEXT UNIQUE
	)`

	if _, err := db.ExecContext(migCtx, schema); err != nil {
		return fmt.Errorf("falha ao executar migracao de schema: %w", err)
	}
	return nil
}

func setupDatabase(ctx context.Context) (*sql.DB, error) {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbSSLMode := os.Getenv("DB_SSLMODE")
	dbUser := os.Getenv("POSTGRES_USER")
	dbPass := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	// Fail Fast: Obriga a injeção da infraestrutura via ambiente
	if dbHost == "" || dbPort == "" || dbUser == "" || dbPass == "" || dbName == "" {
		return nil, fmt.Errorf("variaveis de conexao ausentes (DB_HOST, DB_PORT, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB sao obrigatorias)")
	}
	
	if dbSSLMode == "" {
		dbSSLMode = "disable" // Permitido apenas para desenvolvimento local
	}

	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(dbUser, dbPass),
		Host:     fmt.Sprintf("%s:%s", dbHost, dbPort),
		Path:     "/" + dbName,
		RawQuery: "sslmode=" + dbSSLMode,
	}

	db, err := sql.Open("postgres", u.String())
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexao: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	setupCtx, cancelSetup := context.WithTimeout(ctx, 10*time.Second)
	defer cancelSetup()

	if err := db.PingContext(setupCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro de ping no banco: %w", err)
	}

	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// Retorna uma lista de livros que falharam para serem processados pela DLQ assíncrona
func saveBooksInBatch(ctx context.Context, db *sql.DB, batch []Book) []Book {
	if len(batch) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("falha ao iniciar transacao para insercao em lote", "erro", err)
		return batch
	}
	defer tx.Rollback()

	numFields := 5
	queryStr := "INSERT INTO books (title, price, rating, availability, image_url) VALUES "
	vals := make([]interface{}, 0, len(batch)*numFields)
	placeholders := make([]string, 0, len(batch))

	for i, book := range batch {
		offset := i * numFields
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", offset+1, offset+2, offset+3, offset+4, offset+5))
		vals = append(vals, book.Title, book.Price, book.Rating, book.Availability, book.ImageURL)
	}

	queryStr += strings.Join(placeholders, ", ")
	queryStr += " ON CONFLICT (image_url) DO NOTHING;"

	stmt, err := tx.PrepareContext(ctx, queryStr)
	if err != nil {
		slog.Error("falha ao preparar statement para insercao em lote", "erro", err)
		return batch
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, vals...)
	if err != nil {
		slog.Error("falha ao executar insercao em lote", "erro", err)
		return batch
	}

	if err := tx.Commit(); err != nil {
		slog.Error("falha ao commitar transacao de lote", "erro", err)
		return batch
	}

	return nil
}

func startPipeline(ctx context.Context, db *sql.DB, csvWriter *csv.Writer, fileCSV *os.File, jsonFile *os.File) (chan<- Book, *sync.WaitGroup) {
	booksChan := make(chan Book, 100)
	dlqChan := make(chan Book, 1000)
	dbChan := make(chan Book, 1000) // Canal bufferizado isolado para o banco de dados
	var wg sync.WaitGroup

	var wgDLQ sync.WaitGroup
	wgDLQ.Add(1)
	go func() {
		defer wgDLQ.Done()

		if err := os.MkdirAll("data", 0750); err != nil {
			slog.Error("ERRO FATAL ao criar pasta data para a DLQ", "erro", err)
			os.Exit(1)
		}

		f, err := os.OpenFile("data/dlq.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
		if err != nil {
			slog.Error("ERRO FATAL ao abrir arquivo DLQ", "erro", err)
			os.Exit(1)
		}
		defer f.Close()

		encoder := json.NewEncoder(f)
		for b := range dlqChan {
			if err := encoder.Encode(b); err != nil {
				slog.Error("Erro ao gravar livro na DLQ", "erro", err, "livro", b.Title)
			}
		}
		f.Sync()
	}()

	var wgDBWorker sync.WaitGroup
	var wgDB sync.WaitGroup
	semDB := make(chan struct{}, 5)

	// Goroutine consumidora do Banco de Dados - Roda totalmente isolada do I/O de disco
	if db != nil {
		wgDBWorker.Add(1)
		go func() {
			defer wgDBWorker.Done()

			chunk := make([]Book, 0, 100)
			flushChunk := func() {
				if len(chunk) == 0 {
					return
				}
				batch := make([]Book, len(chunk))
				copy(batch, chunk)
				chunk = chunk[:0]

				semDB <- struct{}{}
				wgDB.Add(1)
				go func(data []Book) {
					defer wgDB.Done()
					defer func() { <-semDB }()

					flushCtx, flushCancel := context.WithTimeout(context.Background(), 15*time.Second)
					failed := saveBooksInBatch(flushCtx, db, data)
					flushCancel()

					for _, fb := range failed {
						select {
						case dlqChan <- fb:
						default:
							slog.Error("CRITICO: Canal DLQ cheio, descartando livro", "livro", fb.Title)
						}
					}
				}(batch)
			}

			for b := range dbChan {
				chunk = append(chunk, b)
				if len(chunk) >= 100 {
					flushChunk()
				}
			}
			flushChunk() // Flush dos elementos remanescentes
		}()
	}

	// Goroutine consumidora principal (I/O de Disco Plano - Sem bloqueios de Rede/Banco)
	wg.Add(1)
	go func() {
		defer wg.Done()

		jsonEncoder := json.NewEncoder(jsonFile)

		defer func() {
			close(dbChan)     // Notifica o dreno do banco de dados
			wgDBWorker.Wait() // Aguarda o dreno terminar
			wgDB.Wait()       // Aguarda transações em andamento
			close(dlqChan)    // Fecha a DLQ com segurança de concorrência
			wgDLQ.Wait()      // Garante escrita do arquivo da DLQ

			csvWriter.Flush()
			if err := csvWriter.Error(); err != nil {
				slog.Error("Erro de gravacao pendente no CSV", "erro", err)
			}
			if err := fileCSV.Sync(); err != nil {
				slog.Error("Erro ao sincronizar fisicamente arquivo CSV no disco", "erro", err)
			}
			if err := jsonFile.Sync(); err != nil {
				slog.Error("Erro ao sincronizar fisicamente arquivo JSONL no disco", "erro", err)
			}
		}()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case b, ok := <-booksChan:
				if !ok {
					return
				}

				// Escrita local e veloz nos arquivos planos de fallback com tratamento rigoroso de erros de I/O
				if err := csvWriter.Write([]string{b.Title, fmt.Sprintf("%.2f", b.Price), strconv.Itoa(b.Rating), b.Availability, b.ImageURL}); err != nil {
					slog.Error("Falha critica de gravacao no arquivo CSV (possivel exaustao de disco)", "erro", err, "livro", b.Title)
				}
				if err := jsonEncoder.Encode(b); err != nil {
					slog.Error("Falha critica de gravacao no arquivo JSONL (possivel exaustao de disco)", "erro", err, "livro", b.Title)
				}

				if db != nil {
					select {
					case dbChan <- b:
					default:
						// Banco lento ou saturado: Desvia não-bloqueante para a DLQ preservando o fluxo
						slog.Warn("Pipeline de banco de dados saturada. Desviando para DLQ de seguranca.", "livro", b.Title)
						select {
						case dlqChan <- b:
						default:
							slog.Error("CRITICO: Fila DLQ saturada. Registro descartado.", "livro", b.Title)
						}
					}
				}

			case <-ticker.C:
				csvWriter.Flush()
			}
		}
	}()

	return booksChan, &wg
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

		var srvWg sync.WaitGroup
	srvWg.Add(1)
	errChan := make(chan error, 1)
	go func() {
		defer srvWg.Done()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Erro FATAL no servidor de Health Check", "erro", err)
			errChan <- err
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			slog.Warn("Sinal de término recebido. Cancelando contexto de requisições...")
		case err := <-errChan:
			slog.Error("Servidor HTTP falhou. Iniciando graceful shutdown...", "erro", err)
		}
		cancel()
	}()

	db, err := setupDatabase(ctx)
	if err != nil {
		slog.Warn("Banco indisponivel. Operando em disco plano.", "erro", err)
	} else {
		defer db.Close()
	}

	if err := os.MkdirAll("data", 0750); err != nil {
		slog.Error("Erro fatal ao criar diretorio data", "erro", err)
		return
	}

	fileCSV, err := os.OpenFile("data/books.csv", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		slog.Error("Erro ao criar arquivo CSV", "erro", err)
		return
	}
	defer fileCSV.Close()

	csvWriter := csv.NewWriter(fileCSV)
	if err := csvWriter.Write([]string{"Title", "Price", "Rating", "Availability", "ImageURL"}); err != nil {
		slog.Error("Erro ao escrever cabecalho CSV", "erro", err)
		return
	}

	// Adotado padrão JSONL em vez de Array manual
	fileJSON, err := os.OpenFile("data/books.jsonl", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		slog.Error("Erro ao criar arquivo JSON", "erro", err)
		return
	}
	defer fileJSON.Close()

	booksChan, wg := startPipeline(ctx, db, csvWriter, fileCSV, fileJSON)

	c := colly.NewCollector(
		colly.UserAgent("ScraperTraineeBot/1.0 (+https://github.com/trainee/scraper)"),
		colly.Async(true),
	)

	c.SetRequestTimeout(30 * time.Second)

	c.OnRequest(func(r *colly.Request) {
		if ctx.Err() != nil {
			r.Abort()
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		slog.Error("Erro de rede do Colly ao tentar ler a pagina", "url", r.Request.URL.String(), "status", r.StatusCode, "erro", err)
	})

	parallelism := 2
	if val := os.Getenv("SCRAPER_PARALLELISM"); val != "" {
		if v, err := strconv.Atoi(val); err == nil {
			parallelism = v
		}
	}

	delay := 1 * time.Second
	if val := os.Getenv("SCRAPER_DELAY"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			delay = d
		}
	}

	if err := c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: parallelism, Delay: delay, RandomDelay: delay / 2}); err != nil {
		slog.Error("Erro ao configurar regras do Colly", "erro", err)
		return
	}

	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		if ctx.Err() != nil {
			return
		}

		title := e.ChildAttr("h3 a", "title")
		priceCleaned, err := cleanPrice(e.ChildText(".price_color"))
		if err != nil {
			slog.Error("Erro ao limpar preco do livro", "livro", title, "erro", err)
			return
		}

		imgURL := e.Request.AbsoluteURL(e.ChildAttr(".image_container img", "src"))
		// Restabelece a idempotência real substituindo URLs vazias por chaves geradas em hash
		if imgURL == "" || strings.HasSuffix(imgURL, "/") {
			hash := sha256.Sum256([]byte(title))
			imgURL = "hash://no-image/" + hex.EncodeToString(hash[:])
		}

		book := Book{
			Title:        title,
			Price:        priceCleaned,
			Rating:       mapRating(e.ChildAttr("p.star-rating", "class")),
			Availability: strings.TrimSpace(e.ChildText(".instock.availability")),
			ImageURL:     imgURL,
		}

		select {
		case booksChan <- book:
		case <-ctx.Done():
			return
		}
	})

	c.OnHTML("li.next a", func(e *colly.HTMLElement) {
		if ctx.Err() != nil {
			return
		}
		if err := e.Request.Visit(e.Attr("href")); err != nil {
			slog.Error("Erro de rede ao buscar proxima pagina", "erro", err)
		}
	})

		slog.Info("Iniciando varredura...")

	// Garante o fechamento seguro em cascata: Colly -> Canais -> Pipeline -> DB -> HTTP Server
	defer func() {
		c.Wait()
		close(booksChan)
		wg.Wait() // Aguarda o encerramento completo do processamento de arquivos e banco de dados

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Erro ao desligar servidor de Health Check", "erro", err)
		}
		srvWg.Wait() // Garante que a goroutine do servidor HTTP finalizou com sucesso
		slog.Info("Graceful Shutdown concluido com sucesso.")
	}()

	if err := c.Visit("https://books.toscrape.com/catalogue/page-1.html"); err != nil {
		slog.Error("Erro fatal ao acessar o site", "erro", err)
	}
}