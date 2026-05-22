package main

import (
	"bufio"
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gocolly/colly/v2"
	_ "github.com/lib/pq"
)

type Book struct {
	Title        string  `json:"title"`
	Price        float64 `json:"price"`
	Rating       int     `json:"rating"`
	Availability string  `json:"availability"`
	ImageURL     string  `json:"image_url"`
}

var priceRegex = regexp.MustCompile(`[0-9]+(?:\.[0-9]{1,2})?`)

var ratingsMap = map[string]int{
	"One":   1,
	"Two":   2,
	"Three": 3,
	"Four":  4,
	"Five":  5,
}

var (
	queryCache = make(map[int]string)
	cacheMu    sync.RWMutex
)

func getInsertQuery(n int) string {
	if n <= 0 {
		return ""
	}

	cacheMu.RLock()
	q, ok := queryCache[n]
	cacheMu.RUnlock()
	if ok {
		return q
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	
	if q, ok = queryCache[n]; ok {
		return q
	}

	var sb strings.Builder
	sb.Grow(n * 35)
	sb.WriteString("INSERT INTO books (title, price, rating, availability, image_url) VALUES ")
	for j := 0; j < n; j++ {
		if j > 0 {
			sb.WriteString(",")
		}
		offset := j * 5
		sb.WriteString("($")
		sb.WriteString(strconv.Itoa(offset + 1))
		sb.WriteString(",$")
		sb.WriteString(strconv.Itoa(offset + 2))
		sb.WriteString(",$")
		sb.WriteString(strconv.Itoa(offset + 3))
		sb.WriteString(",$")
		sb.WriteString(strconv.Itoa(offset + 4))
		sb.WriteString(",$")
		sb.WriteString(strconv.Itoa(offset + 5))
		sb.WriteString(")")
	}
	sb.WriteString(" ON CONFLICT (image_url) DO NOTHING")
	queryCache[n] = sb.String()
	return queryCache[n]
}

func cleanPrice(priceStr string) (float64, error) {
	match := priceRegex.FindString(priceStr)
	if match == "" {
		return 0.0, fmt.Errorf("padrao de preco nao encontrado no texto: %s", priceStr)
	}
	val, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0.0, fmt.Errorf("erro ao converter preco para float: %w", err)
	}
	return val, nil
}

func mapRating(classStr string) int {
	parts := strings.Fields(classStr)
	for _, part := range parts {
		if val, ok := ratingsMap[part]; ok {
			return val
		}
	}
	slog.Warn("Nao foi possivel mapear a classe de rating. Valor retornado como 0.", "classe", classStr)
	return 0
}

func setupDatabase() (*sql.DB, error) {
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
		Scheme: "postgres",
		User:   url.UserPassword(dbUser, dbPass),
		Host:   fmt.Sprintf("%s:%s", dbHost, dbPort),
		Path:   "/" + dbName,
	}
	q := u.Query()
	q.Set("sslmode", dbSSLMode)
	u.RawQuery = q.Encode()

	db, err := sql.Open("postgres", u.String())
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexao: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro de ping no banco: %w", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS books (
		id SERIAL PRIMARY KEY, title TEXT, price NUMERIC, rating INT, availability TEXT, image_url TEXT UNIQUE
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao criar tabela: %w", err)
	}

	return db, nil
}

func saveBooksInBatch(ctx context.Context, db *sql.DB, books []Book) {
	if len(books) == 0 {
		return
	}

	for i := 0; i < len(books); i += 100 {
		end := i + 100
		if end > len(books) {
			end = len(books)
		}
		batch := books[i:end]
		n := len(batch)

		valueArgs := make([]interface{}, 0, n*5)
		for _, b := range batch {
			valueArgs = append(valueArgs, b.Title, b.Price, b.Rating, b.Availability, b.ImageURL)
		}

		dbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)

		stmtStr := getInsertQuery(n)
		if stmtStr == "" {
			slog.Warn("Query de bulk vazia. Abortando lote.")
			cancel() 
			continue
		}

		if _, err := db.ExecContext(dbCtx, stmtStr, valueArgs...); err != nil {
			slog.Error("Erro no bulk insert de lote. Iniciando fallback de transação individual...", "erro", err)
			
			tx, txErr := db.BeginTx(dbCtx, nil)
			if txErr != nil {
				slog.Error("Erro ao abrir transacao de fallback", "erro", txErr)
				cancel()
				continue
			}

			stmt, prepErr := tx.PrepareContext(dbCtx, "INSERT INTO books (title, price, rating, availability, image_url) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (image_url) DO NOTHING")
			if prepErr != nil {
				slog.Error("Erro ao preparar statement de fallback", "erro", prepErr)
				_ = tx.Rollback()
				cancel()
				continue
			}

			// CORRIGIDO: Iterando estritamente sobre o 'batch' atual em vez de 'books' (evita repetições indevidas no fallback)
			for _, b := range batch {
				if dbCtx.Err() != nil {
					slog.Warn("Cancelando inserções individuais de fallback: contexto de rede expirado.")
					break
				}
				if _, err := stmt.ExecContext(dbCtx, b.Title, b.Price, b.Rating, b.Availability, b.ImageURL); err != nil {
					slog.Error("Erro ao salvar livro individual no fallback (Ignorado)", "livro", b.Title, "erro", err)
				}
			}
			stmt.Close()
			if err := tx.Commit(); err != nil {
				slog.Error("Erro ao comitar transacao de fallback", "erro", err)
			}
		}
		cancel() 
	}
}

func startPipeline(ctx context.Context, db *sql.DB, csvWriter *csv.Writer, fileCSV *os.File, jsonFile *os.File) (chan<- Book, *sync.WaitGroup) {
	booksChan := make(chan Book, 100)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		
		jsonWriter := bufio.NewWriter(jsonFile)

		defer func() {
			csvWriter.Flush()
			if err := csvWriter.Error(); err != nil {
				slog.Error("Erro de gravacao pendente no CSV", "erro", err)
			}
			if err := fileCSV.Sync(); err != nil {
				slog.Error("Erro ao forcar sincronizacao fisica do arquivo CSV (fsync)", "erro", err)
			}

			// CORRIGIDO: Escreve o fechamento do array JSON no final do arquivo
			if _, err := jsonWriter.WriteString("\n]\n"); err != nil {
				slog.Error("Erro ao finalizar array JSON", "erro", err)
			}
			if err := jsonWriter.Flush(); err != nil {
				slog.Error("Erro ao dar flush no buffer do JSON", "erro", err)
			}
			if err := jsonFile.Sync(); err != nil {
				slog.Error("Erro ao forcar sincronizacao fisica do arquivo JSON (fsync)", "erro", err)
			}
		}()

		// CORRIGIDO: Inicializa o arquivo como um array JSON válido sem carregar tudo em RAM
		if _, err := jsonWriter.WriteString("[\n"); err != nil {
			slog.Error("Erro ao iniciar array no JSON", "erro", err)
		}

		chunk := make([]Book, 0, 100) 
		isFirstJSON := true

		for b := range booksChan {
			if err := csvWriter.Write([]string{b.Title, fmt.Sprintf("%.2f", b.Price), strconv.Itoa(b.Rating), b.Availability, b.ImageURL}); err != nil {
				slog.Error("Erro ao escrever linha no CSV", "erro", err)
			}

			// CORRIGIDO: Transforma o fluxo contínuo de dados em um array estruturado válido
			if !isFirstJSON {
				if _, err := jsonWriter.WriteString(",\n"); err != nil {
					slog.Error("Erro ao injetar delimitador no JSON", "erro", err)
				}
			}
			isFirstJSON = false

			bBytes, err := json.MarshalIndent(b, "  ", "  ")
			if err != nil {
				slog.Error("Erro ao codificar JSON do livro", "erro", err)
			} else {
				if _, err := jsonWriter.Write(bBytes); err != nil {
					slog.Error("Erro ao persistir bloco JSON", "erro", err)
				}
			}

			if db != nil {
				chunk = append(chunk, b)
				if len(chunk) >= 100 {
					flushCtx, flushCancel := context.WithTimeout(context.Background(), 15*time.Second)
					saveBooksInBatch(flushCtx, db, chunk)
					flushCancel()
					chunk = chunk[:0] 
				}
			}
		}

		if db != nil && len(chunk) > 0 {
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
			saveBooksInBatch(flushCtx, db, chunk)
			flushCancel()
		}
	}()

	return booksChan, &wg
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

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

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Erro no servidor de Health Check", "erro", err)
		}
	}()

	db, err := setupDatabase()
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

	// CORRIGIDO: Salvando como arquivo '.json' estrito e bem formatado
	fileJSON, err := os.OpenFile("data/books.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		slog.Error("Erro ao criar arquivo JSON", "erro", err)
		return
	}
	defer fileJSON.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	booksChan, wg := startPipeline(ctx, db, csvWriter, fileCSV, fileJSON)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Warn("Sinal de término recebido. Cancelando contexto de requisições...")
		cancel() 
	}()

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
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
		if v, err := strconv.Atoi(val); err == nil { parallelism = v }
	}

	delay := 1 * time.Second
	if val := os.Getenv("SCRAPER_DELAY"); val != "" {
		if d, err := time.ParseDuration(val); err == nil { delay = d }
	}

	if err := c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: parallelism, RandomDelay: delay}); err != nil {
		slog.Error("Erro ao configurar regras do Colly", "erro", err)
		return
	}

	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		if ctx.Err() != nil {
			return
		}

		priceCleaned, err := cleanPrice(e.ChildText(".price_color"))
		if err != nil {
			slog.Error("Erro ao limpar preco do livro", "livro", e.ChildAttr("h3 a", "title"), "erro", err)
			return
		}

		book := Book{
			Title:        e.ChildAttr("h3 a", "title"),
			Price:        priceCleaned,
			Rating:       mapRating(e.ChildAttr("p.star-rating", "class")),
			Availability: strings.TrimSpace(e.ChildText(".instock.availability")),
			ImageURL:     e.Request.AbsoluteURL(e.ChildAttr(".image_container img", "src")),
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

	c.Wait()         
	close(booksChan) 
	wg.Wait()        

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Erro ao desligar servidor de Health Check", "erro", err)
	}
}
