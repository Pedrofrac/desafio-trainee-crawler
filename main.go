package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

// Book representa a estrutura dos dados
type Book struct {
	Title        string  `json:"title"`
	Price        float64 `json:"price"` // Mudamos para número (float64)
	Rating       int     `json:"rating"`
	Availability string  `json:"availability"`
	ImageURL     string  `json:"image_url"`
}

// mapRating converte a classe CSS para número
func mapRating(classStr string) int {
	ratings := map[string]int{
		"One":   1,
		"Two":   2,
		"Three": 3,
		"Four":  4,
		"Five":  5,
	}
	parts := strings.Split(classStr, " ")
	if len(parts) == 2 {
		return ratings[parts[1]]
	}
	return 0
}

// cleanPrice remove os símbolos de moeda e converte para float
func cleanPrice(priceStr string) float64 {
	// Remove o símbolo de libra e o caractere 'Â' que costuma bugar no terminal
	clean := strings.ReplaceAll(priceStr, "£", "")
	clean = strings.ReplaceAll(clean, "Â", "")
	clean = strings.TrimSpace(clean)

	val, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0.0
	}
	return val
}

func main() {
	c := colly.NewCollector(
		colly.AllowedDomains("books.toscrape.com"),
		colly.UserAgent("Trainee-Scraper-Bot/1.0 (+https://meu-portfolio.com)"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*books.toscrape.com*",
		Delay:       1 * time.Second,
		RandomDelay: 500 * time.Millisecond,
	})

	var books []Book

	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		title := e.ChildAttr("h3 a", "title")
		
		// Usamos a nossa função nova para limpar o preço
		priceRaw := e.ChildText(".price_color")
		price := cleanPrice(priceRaw)
		
		availability := strings.TrimSpace(e.ChildText(".instock.availability"))
		imageURL := e.ChildAttr(".image_container img", "src")
		ratingClass := e.ChildAttr("p.star-rating", "class")

		book := Book{
			Title:        title,
			Price:        price,
			Rating:       mapRating(ratingClass),
			Availability: availability,
			ImageURL:     e.Request.AbsoluteURL(imageURL),
		}

		books = append(books, book)
		fmt.Printf("Extraído: %s | Preço: %.2f | Estrelas: %d\n", book.Title, book.Price, book.Rating)
	})

	c.OnHTML("li.next a", func(e *colly.HTMLElement) {
		absoluteURL := e.Request.AbsoluteURL(e.Attr("href"))
		fmt.Printf("\n---> Indo para a próxima página: %s\n\n", absoluteURL)
		err := c.Visit(absoluteURL)
		if err != nil {
			log.Println("Erro ao visitar próxima página:", err)
		}
	})

	// Inicia o Crawler
	err := c.Visit("https://books.toscrape.com/catalogue/page-1.html")
	if err != nil {
		log.Fatal("Erro fatal ao iniciar:", err)
	}

	fmt.Printf("\n=== RESUMO ===\nTotal de livros extraídos: %d\n", len(books))

	// ---- ETAPA 2: SALVAR OS DADOS ----
	os.MkdirAll("data", os.ModePerm) // Cria a pasta "data"

	// Salva JSON
	fileJSON, _ := os.Create("data/books.json")
	defer fileJSON.Close()
	encoder := json.NewEncoder(fileJSON)
	encoder.SetIndent("", "  ")
	encoder.Encode(books)
	fmt.Println("✅ Arquivo books.json salvo na pasta data!")

	// Salva CSV
	fileCSV, _ := os.Create("data/books.csv")
	defer fileCSV.Close()
	writer := csv.NewWriter(fileCSV)
	defer writer.Flush()
	
	// Cabeçalho do CSV
	writer.Write([]string{"Title", "Price", "Rating", "Availability", "ImageURL"})
	// Linhas do CSV
	for _, b := range books {
		writer.Write([]string{
			b.Title,
			fmt.Sprintf("%.2f", b.Price),
			strconv.Itoa(b.Rating),
			b.Availability,
			b.ImageURL,
		})
	}
	fmt.Println("✅ Arquivo books.csv salvo na pasta data!")
}