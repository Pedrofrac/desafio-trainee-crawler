package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

func cleanPrice(priceStr string) float64 {
	match := priceRegex.FindString(priceStr)
	val, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0.0
	}
	return val
}

func mapRating(classStr string) int {
	parts := strings.Fields(classStr)
	for _, part := range parts {
		if val, ok := ratingsMap[part]; ok {
			return val
		}
	}
	return 0
}

func setupDatabase() (*sql.DB, error) {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" { dbHost = "localhost" }
	
	dbUser := os.Getenv("POSTGRES_USER")
	dbPass := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")

	if dbUser == "" || dbPass == "" || dbName == "" {
		return nil, fmt.Errorf("credenciais de banco de dados ausentes (defina POSTGRES_USER, POSTGRES_PASSWORD e POSTGRES_DB)")
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", dbUser, dbPass, dbHost, dbName)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir conexao: %w", err)
	}

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

func startPipeline(db *sql.DB, csvWriter *csv.Writer, jsonFile *os.File) (chan<- Book, *sync.WaitGroup) {
	booksChan := make(chan Book, 100)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		
		var tx *sql.Tx
		var stmt *sql.Stmt
		var err error

		if db != nil {
			tx, err = db.Begin()
			if err != nil {
				log.Printf("Erro ao iniciar transacao: %v", err)
			}
		}

		if tx != nil {
			defer tx.Rollback()
			stmt, err = tx.Prepare("INSERT INTO books (title, price, rating, availability, image_url) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (image_url) DO NOTHING")
			if err != nil {
				log.Printf("Erro ao preparar statement: %v", err)
			} else {
				defer stmt.Close()
			}
		}

		jsonWriter := bufio.NewWriter(jsonFile)

		if _, err := jsonWriter.Write([]byte("[\n")); err != nil {
			log.Printf("Erro ao inicializar arquivo JSON: %v", err)
		}

		first := true
		totalBooks := 0

		for b := range booksChan {
			totalBooks++

			if err := csvWriter.Write([]string{b.Title, fmt.Sprintf("%.2f", b.Price), strconv.Itoa(b.Rating), b.Availability, b.ImageURL}); err != nil {
				log.Printf("Erro ao escrever CSV: %v", err)
			}

			if !first {
				if _, err := jsonWriter.Write([]byte(",\n")); err != nil {
					log.Printf("Erro ao escrever delimitador JSON: %v", err)
				}
			} else {
				first = false
			}

			data, err := json.Marshal(b)
			if err != nil {
				log.Printf("Erro ao serializar JSON: %v", err)
			} else {
				if _, err := jsonWriter.Write(data); err != nil {
					log.Printf("Erro ao gravar dados JSON no buffer: %v", err)
				}
			}

			if stmt != nil {
				if _, err := stmt.Exec(b.Title, b.Price, b.Rating, b.Availability, b.ImageURL); err != nil {
					log.Printf("Erro isolado no DB (Linha ignorada): %v", err)
				}
			}
		}

		csvWriter.Flush()
		
		// Correcao: Validacao de erros pendentes de gravacao no buffer do CSV
		if err := csvWriter.Error(); err != nil {
			log.Printf("Erro de gravacao pendente no CSV: %v", err)
		}
		
		if _, err := jsonWriter.Write([]byte("\n]\n")); err != nil {
			log.Printf("Erro ao escrever fechamento JSON: %v", err)
		}

		if err := jsonWriter.Flush(); err != nil {
			log.Printf("Erro ao dar flush no buffer do JSON: %v", err)
		}

		if tx != nil && stmt != nil {
			if err := tx.Commit(); err != nil {
				log.Printf("Erro ao comitar transacao de DB: %v", err)
			}
		}
	}()

	return booksChan, &wg
}

func main() {
	db, err := setupDatabase()
	if err != nil {
		log.Printf("Aviso: Banco indisponivel. Operando apenas com I/O de disco. Detalhe: %v", err)
	} else {
		defer db.Close()
	}

	if err := os.MkdirAll("data", 0750); err != nil {
		log.Printf("Erro fatal ao criar diretorio data: %v", err)
		return
	}

	fileCSV, err := os.Create("data/books.csv")
	if err != nil {
		log.Printf("Erro ao criar arquivo CSV: %v", err)
		return
	}
	defer fileCSV.Close()

	csvWriter := csv.NewWriter(fileCSV)
	if err := csvWriter.Write([]string{"Title", "Price", "Rating", "Availability", "ImageURL"}); err != nil {
		log.Printf("Erro ao escrever cabecalho CSV: %v", err)
		return
	}

	fileJSON, err := os.Create("data/books.json")
	if err != nil {
		log.Printf("Erro ao criar arquivo JSON: %v", err)
		return
	}
	defer fileJSON.Close()

	booksChan, wg := startPipeline(db, csvWriter, fileJSON)

	c := colly.NewCollector(
		colly.AllowedDomains("books.toscrape.com"),
		colly.UserAgent("Scraper-Bot/9.0"),
		colly.Async(true),
	)

	if err := c.Limit(&colly.LimitRule{DomainGlob: "*books.toscrape.com*", Parallelism: 2, RandomDelay: 1 * time.Second}); err != nil {
		log.Printf("Erro ao configurar regras do Colly: %v", err)
		return
	}

	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		book := Book{
			Title:        e.ChildAttr("h3 a", "title"),
			Price:        cleanPrice(e.ChildText(".price_color")),
			Rating:       mapRating(e.ChildAttr("p.star-rating", "class")),
			Availability: strings.TrimSpace(e.ChildText(".instock.availability")),
			ImageURL:     e.Request.AbsoluteURL(e.ChildAttr(".image_container img", "src")),
		}
		booksChan <- book 
	})

	c.OnHTML("li.next a", func(e *colly.HTMLElement) {
		if err := e.Request.Visit(e.Attr("href")); err != nil {
			log.Printf("Erro de rede ao buscar proxima pagina: %v", err)
		}
	})

	log.Println("Iniciando varredura...")
	if err := c.Visit("https://books.toscrape.com/catalogue/page-1.html"); err != nil {
		log.Printf("Erro fatal ao acessar o site: %v", err)
		return
	}

	c.Wait()         
	close(booksChan) 
	wg.Wait()        
}
