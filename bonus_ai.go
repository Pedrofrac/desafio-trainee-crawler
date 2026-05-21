package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gocolly/colly/v2"
)

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("ERRO: Variavel GEMINI_API_KEY nao configurada.")
	}

	c := colly.NewCollector(colly.AllowedDomains("books.toscrape.com"))
	var dynamicBookURL string

	c.OnHTML("article.product_pod h3 a", func(e *colly.HTMLElement) {
		if dynamicBookURL == "" {
			dynamicBookURL = e.Request.AbsoluteURL(e.Attr("href"))
		}
	})

	if err := c.Visit("https://books.toscrape.com/"); err != nil {
		log.Fatalf("Erro ao visitar a home: %v", err)
	}

	if dynamicBookURL == "" {
		log.Fatal("Nenhum livro encontrado na pagina inicial.")
	}
	fmt.Printf("Link dinamico capturado: %s\n", dynamicBookURL)

	var description string
	c2 := colly.NewCollector(colly.AllowedDomains("books.toscrape.com"))
	c2.OnHTML("#content_inner > article > p", func(e *colly.HTMLElement) {
		description = e.Text
	})

	if err := c2.Visit(dynamicBookURL); err != nil {
		log.Fatalf("Erro ao visitar a pagina do livro: %v", err)
	}

	// Evita desperdicio de rede e tokens chamando a API com texto vazio
	if description == "" {
		log.Fatal("ERRO: Descricao extraida do livro esta vazia. Execucao abortada.")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)
	prompt := fmt.Sprintf(`Leia a seguinte sinopse de livro e extraia o Assunto Principal e o Sentimento em formato JSON {"assunto": "", "sentimento": ""}. Texto: "%s"`, description)
	
	reqBodyMap := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]interface{}{{"text": prompt}}}},
		"generationConfig": map[string]interface{}{"responseMimeType": "application/json"},
	}
	
	reqBody, err := json.Marshal(reqBodyMap)
	if err != nil {
		log.Fatalf("Erro ao serializar payload JSON: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("Erro ao consultar API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Erro ao ler corpo da resposta: %v", err)
	}

	var geminiResp struct {
		Candidates []struct { Content struct { Parts []struct { Text string `json:"text"` } `json:"parts"` } `json:"content"` } `json:"candidates"`
	}
	
	if err := json.Unmarshal(body, &geminiResp); err != nil || len(geminiResp.Candidates) == 0 {
		log.Fatalf("Erro decode AI: %v. Resposta bruta: %s", err, string(body))
	}

	fmt.Println("Resposta Estruturada da IA:", geminiResp.Candidates[0].Content.Parts[0].Text)
}
