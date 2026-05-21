package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gocolly/colly/v2"
)

// Estrutura para os dados BRUTOS e sujos vindo do HTML
type RawBook struct {
	Title           string `json:"title"`
	PriceRaw        string `json:"price_raw"`
	RatingRaw       string `json:"rating_raw"`
	AvailabilityRaw string `json:"availability_raw"`
	ImageURL        string `json:"image_url"`
}

// Estrutura para receber os dados LIMPOS pela IA
type CleanBook struct {
	Title        string  `json:"title"`
	Price        float64 `json:"price"`
	Rating       int     `json:"rating"`
	Availability string  `json:"availability"`
	ImageURL     string  `json:"image_url"`
}

func readAPIKey() (string, error) {
	data, err := os.ReadFile("api/key.txt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Função que envia os dados brutos reais para o Gemini 2.5 limpar
func cleanBooksWithGemini(apiKey string, rawBooks []RawBook) ([]CleanBook, error) {
	// Usando o modelo Gemini 2.5 Flash oficial como solicitado!
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

	rawJSON, _ := json.Marshal(rawBooks)

	prompt := fmt.Sprintf(`Você é um limpador de dados. Eu acabei de raspar 3 livros de um site real e eles vieram com os dados brutos muito sujos do HTML. 

Sua tarefa é ler esse JSON sujo de entrada e me devolver APENAS um array JSON limpo e estruturado convertendo:
- "price" para um número decimal (float64) puro (remova símbolos de moeda como '£' ou 'Â').
- "rating" para um número inteiro (de 1 a 5), lendo o texto da classe CSS (ex: "star-rating Three" vira 3, "star-rating One" vira 1).
- "availability" para um texto curto e limpo (ex: "In stock").

JSON sujo para você limpar:
%s`, string(rawJSON))

	requestBody, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json", // Exige resposta em JSON
		},
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var geminiResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	err = json.Unmarshal(body, &geminiResponse)
	if err != nil || len(geminiResponse.Candidates) == 0 {
		return nil, fmt.Errorf("Erro ao ler resposta do Gemini: %s", string(body))
	}

	jsonText := geminiResponse.Candidates[0].Content.Parts[0].Text
	
	var cleanBooks []CleanBook
	err = json.Unmarshal([]byte(jsonText), &cleanBooks)
	if err != nil {
		return nil, err
	}

	return cleanBooks, nil
}

func main() {
	fmt.Println("🔑 Carregando chave da API...")
	apiKey, err := readAPIKey()
	if err != nil {
		log.Fatal("Erro ao ler chave:", err)
	}

	fmt.Println("🕸️  Iniciando Colly para raspar os 3 primeiros livros reais do site...")

	c := colly.NewCollector(
		colly.AllowedDomains("books.toscrape.com"),
	)

	var rawBooks []RawBook
	count := 0

	// Captura os dados exatamente como vieram do HTML (sujos)
	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		if count < 3 {
			title := e.ChildAttr("h3 a", "title")
			priceRaw := e.ChildText(".price_color")
			availabilityRaw := e.ChildText(".instock.availability")
			imageURL := e.Request.AbsoluteURL(e.ChildAttr(".image_container img", "src"))
			ratingRaw := e.ChildAttr("p.star-rating", "class")

			rawBooks = append(rawBooks, RawBook{
				Title:           title,
				PriceRaw:        priceRaw,
				RatingRaw:       ratingRaw,
				AvailabilityRaw: availabilityRaw,
				ImageURL:        imageURL,
			})
			count++
		}
	})

	// Visita a página inicial real do site
	err = c.Visit("https://books.toscrape.com/catalogue/page-1.html")
	if err != nil {
		log.Fatal("Erro ao raspar o site:", err)
	}

	fmt.Printf("📦 Capturados %d livros reais do HTML. Enviando para o Gemini 2.5 Flash limpar...\n", len(rawBooks))

	// Envia os dados reais do site para a IA limpar
	cleanBooks, err := cleanBooksWithGemini(apiKey, rawBooks)
	if err != nil {
		log.Fatal("❌ Erro ao processar dados com o Gemini 2.5: ", err)
	}

	fmt.Println("\n✨ Dados Reais extraídos do site e LIMPOS de forma inteligente pelo Gemini 2.5:")
	for i, book := range cleanBooks {
		fmt.Printf("\n--- Livro %d ---\n", i+1)
		fmt.Printf("📖 Título: %s\n", book.Title)
		fmt.Printf("💵 Preço: R$ %.2f\n", book.Price)
		fmt.Printf("⭐ Rating: %d Estrelas\n", book.Rating)
		fmt.Printf("📦 Status: %s\n", book.Availability)
	}
}