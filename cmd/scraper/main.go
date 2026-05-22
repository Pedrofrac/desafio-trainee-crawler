package main

import (
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
	// Remover espacos iniciais/finais redundantes
	trimmed := strings.TrimSpace(priceStr)
	if trimmed == "" {
		return 0.0, fmt.Errorf("string de preco vazia")
	}

	// Filtrar apenas caracteres numericos, ponto e virgula
	var sb strings.Builder
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			sb.WriteRune(r)
		}
	}
	cleaned := sb.String()
	if len(cleaned) == 0 {
		return 0.0, fmt.Errorf("nenhum caractere numerico ou separador encontrado em %q", priceStr)
	}

	// Localizar o indice do ultimo caractere nao alfanumerico (ponto ou virgula)
	lastIdx := -1
	for i := len(cleaned) - 1; i >= 0; i-- {
		if cleaned[i] == '.' || cleaned[i] == ',' {
			lastIdx = i
			break
		}
	}

	var integerPart, decimalPart string
	if lastIdx != -1 {
		// Parte inteira e tudo a esquerda do ultimo separador
		left := cleaned[:lastIdx]
		// Parte decimal e tudo a direita do ultimo separador
		right := cleaned[lastIdx+1:]

		// Limpar a parte inteira de outros separadores (milhar) mantendo apenas digitos
		var intSb strings.Builder
		for _, r := range left {
			if r >= '0' && r <= '9' {
				intSb.WriteRune(r)
			}
		}
		integerPart = intSb.String()

		// Limpar a parte decimal para conter apenas digitos
		var decSb strings.Builder
		for _, r := range right {
			if r >= '0' && r <= '9' {
				decSb.WriteRune(r)
			}
		}
		decimalPart = decSb.String()
	} else {
		// Sem separador decimal explicito, trata a string inteira limpa como inteiro
		var intSb strings.Builder
		for _, r := range cleaned {
			if r >= '0' && r <= '9' {
				intSb.WriteRune(r)
			}
		}
		integerPart = intSb.String()
	}

	// Validacao final da reconstrucao numerica
	if integerPart == "" && decimalPart == "" {
		return 0.0, fmt.Errorf("falha ao sanitizar partes inteira e decimal de %q", priceStr)
	}

	if integerPart == "" {
		integerPart = "0"
	}

	finalStr := integerPart
	if decimalPart != "" {
		finalStr += "." + decimalPart
	}

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
	if dbHost == "" { dbHost = "localhost" }
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" { dbPort = "5432" }
	dbSSLMode := os.Getenv("DB_SSLMODE")
	if dbSSLMode == "" { dbSSLMode = "disable" }

	dbUser := os.Getenv("POSTGRES_USER")
	dbPass := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	if dbUser == "" || dbPass == "" || dbName == "" {
		return nil, fmt.Errorf("credenciais de banco de dados ausentes no ambiente")
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
	var wg sync.WaitGroup

	var wgDLQ sync.WaitGroup
	wgDLQ.Add(1)
	go func() {
		defer wgDLQ.Done()

		if err := os.MkdirAll("data", 0750); err != nil {
			slog.Error("ERRO FATAL ao criar pasta data para a DLQ. Abortando execucao.", "erro", err)
			os.Exit(1)
		}

		f, err := os.OpenFile("data/dlq.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
		if err != nil {
			slog.Error("ERRO FATAL ao abrir arquivo DLQ. Abortando processo de execucao para evitar perda de dados.", "erro", err)
			os.Exit(1)
		}
		defer f.Close()

		encoder := json.NewEncoder(f)
		for b := range dlqChan {
			if err := encoder.Encode(b); err != nil {
				slog.Error("Erro ao gravar livro na DLQ", "erro", err, "livro", b.Title)
			}
		}
		if err := f.Sync(); err != nil {
			slog.Error("Erro ao aplicar fsync na DLQ", "erro", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		jsonEncoder := json.NewEncoder(jsonFile)

		var wgDB sync.WaitGroup
		semDB := make(chan struct{}, 5) // Semáforo limitador para mitigar Memory Leak de Goroutines

		defer func() {
			wgDB.Wait()
			close(dlqChan)
			wgDLQ.Wait()

			if err := jsonFile.Sync(); err != nil {
				slog.Error("Erro ao forcar sincronizacao fisica do arquivo JSON", "erro", err)
			}

			csvWriter.Flush()
			if err := csvWriter.Error(); err != nil {
				slog.Error("Erro de gravacao pendente no CSV", "erro", err)
			}
			if err := fileCSV.Sync(); err != nil {
				slog.Error("Erro ao forcar sincronizacao fisica do arquivo CSV", "erro", err)
			}
		}()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		chunk := make([]Book, 0, 100)

		for {
			select {
			case b, ok := <-booksChan:
				if !ok {
					if db != nil && len(chunk) > 0 {
						batch := make([]Book, len(chunk))
						copy(batch, chunk)

						semDB <- struct{}{}
						wgDB.Add(1)
						go func(data []Book) {
							defer wgDB.Done()
							flushCtx, flushCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
							failed := saveBooksInBatch(flushCtx, db, data)
							flushCancel()
							<-semDB // Liberado ANTES da DLQ para prevenir Deadlock lógico

							for _, fb := range failed {
								dlqChan <- fb
							}
						}(batch)
					}
					return
				}

				if err := csvWriter.Write([]string{b.Title, fmt.Sprintf("%.2f", b.Price), strconv.Itoa(b.Rating), b.Availability, b.ImageURL}); err != nil {
					slog.Error("Erro ao escrever linha no CSV", "erro", err)
				}

				if err := jsonEncoder.Encode(b); err != nil {
					slog.Error("Erro ao persistir bloco JSONL", "erro", err)
				}

				if db != nil {
					chunk = append(chunk, b)
					if len(chunk) >= 100 {
						batch := make([]Book, len(chunk))
						copy(batch, chunk)
						chunk = chunk[:0]

						semDB <- struct{}{}
						wgDB.Add(1)
						go func(data []Book) {
							defer wgDB.Done()
							flushCtx, flushCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
							failed := saveBooksInBatch(flushCtx, db, data)
							flushCancel()
							<-semDB // Liberado ANTES da DLQ

							for _, fb := range failed {
								dlqChan <- fb
							}
						}(batch)
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

	errChan := make(chan error, 1)
	go func() {
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

	if err := c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: parallelism, RandomDelay: delay}); err != nil {
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
			imgURL = "no-image-url:title:" + url.PathEscape(title)
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
	if err := c.Visit("https://books.toscrape.com/catalogue/page-1.html"); err != nil {
		slog.Error("Erro fatal ao acessar o site", "erro", err)
		return
	}

	go func() {
		c.Wait()
		close(booksChan)
	}()
	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Erro ao desligar servidor de Health Check", "erro", err)
	}
}