package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

type ExtractedData map[string]any

func cleanMarkdownJSON(jsonText string) string {
	re := regexp.MustCompile(`(?s)\x60\x60\x60(?:json)?(.*?)\x60\x60\x60`)
	if match := re.FindStringSubmatch(jsonText); len(match) > 1 {
		jsonText = match[1]
	}
	return strings.TrimSpace(jsonText)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("Falha critica na execucao da IA", "erro", err)
		os.Exit(1) 
	}
}

func run() error {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("variavel GEMINI_API_KEY nao configurada")
	}

	retryLimit := 3
	if limitStr := os.Getenv("API_RETRY_LIMIT"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			retryLimit = l
		} else {
			slog.Warn("Falha ao analisar API_RETRY_LIMIT. Usando padrao de 3.", "erro", err, "input", limitStr)
		}
	}

	backoff := 2 * time.Second
	if backoffStr := os.Getenv("API_BACKOFF_DURATION"); backoffStr != "" {
		if d, err := time.ParseDuration(backoffStr); err == nil {
			backoff = d
		}
	}

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)
	if err := c.Limit(&colly.LimitRule{DomainGlob: "*", Delay: 1 * time.Second}); err != nil {
		return fmt.Errorf("erro no limit do coletor 1: %w", err)
	}

	var dynamicBookURL string
	c.OnHTML("article.product_pod h3 a", func(e *colly.HTMLElement) {
		if dynamicBookURL == "" {
			dynamicBookURL = e.Request.AbsoluteURL(e.Attr("href"))
		}
	})

	if err := c.Visit("https://books.toscrape.com/"); err != nil {
		return fmt.Errorf("erro ao visitar a home: %w", err)
	}

	if dynamicBookURL == "" {
		return fmt.Errorf("nenhum livro localizado na varredura da pagina inicial")
	}
	slog.Info("Link dinamico capturado", "url", dynamicBookURL)

	var description string
	c2 := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)
	if err := c2.Limit(&colly.LimitRule{DomainGlob: "*", Delay: 1 * time.Second}); err != nil {
		return fmt.Errorf("erro no limit do coletor 2: %w", err)
	}

	c2.OnHTML("#content_inner > article > p", func(e *colly.HTMLElement) {
		description = e.Text
	})

	if err := c2.Visit(dynamicBookURL); err != nil {
		return fmt.Errorf("erro ao visitar a pagina do livro: %w", err)
	}

	if description == "" {
		return fmt.Errorf("descricao extraida do livro esta vazia")
	}

	sanitizedDescription := strings.ReplaceAll(description, "\"", "'")

	baseURL := os.Getenv("GEMINI_API_URL")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("erro ao analisar URL base: %w", err)
	}
	query := parsedURL.Query()
	query.Set("key", apiKey)
	parsedURL.RawQuery = query.Encode()

	prompt := fmt.Sprintf(`Leia a seguinte sinopse de livro e extraia o Assunto Principal e o Sentimento em formato JSON {"assunto": "", "sentimento": ""}. Texto: "%s"`, sanitizedDescription)
	
	reqBodyMap := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]interface{}{{"text": prompt}}}},
		"generationConfig": map[string]interface{}{"responseMimeType": "application/json"},
	}
	
	reqBody, err := json.Marshal(reqBodyMap)
	if err != nil {
		return fmt.Errorf("erro ao serializar payload JSON: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	var resp *http.Response
	var respErr error
	var body []byte

	for i := 1; i <= retryLimit; i++ {
		req, err := http.NewRequestWithContext(ctx, "POST", parsedURL.String(), bytes.NewBuffer(reqBody))
		if err != nil {
			return fmt.Errorf("erro ao criar requisicao HTTP: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, respErr = httpClient.Do(req)
		if respErr == nil && resp.StatusCode == http.StatusOK {
			break
		}

		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			resp.Body.Close()
		}
		slog.Warn("Tentativa de conexao falhou. Iniciando recuo exponencial...", "tentativa", i, "limite", retryLimit, "erro", respErr, "status", statusCode, "espera", backoff)
		
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("tempo limite de execucao do pipeline expirado")
		case <-timer.C:
		}
		backoff *= 2 
	}

	if respErr != nil || resp == nil || resp.StatusCode != http.StatusOK {
		return fmt.Errorf("erro persistente na API do Gemini apos o limite de tentativas de retry")
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta do corpo do Gemini: %w", err)
	}

	var geminiResp struct {
		Candidates []struct { Content struct { Parts []struct { Text string `json:"text"` } `json:"parts"` } `json:"content"` } `json:"candidates"`
	}
	
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		bodySnippet := string(body)
		if len(bodySnippet) > 100 {
			bodySnippet = bodySnippet[:100] + "... [truncado]"
		}
		return fmt.Errorf("erro de desserializacao JSON da resposta bruta da API: %w. Resposta: %s", err, bodySnippet)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("Gemini retornou uma estrutura de resposta vazia")
	}

	jsonText := cleanMarkdownJSON(geminiResp.Candidates[0].Content.Parts[0].Text)

	var extractedData ExtractedData
	if err := json.Unmarshal([]byte(jsonText), &extractedData); err != nil {
		return fmt.Errorf("falha critica ao converter JSON limpo da IA: %w", err)
	}

	slog.Info("Dados de NLP processados com sucesso pela IA", "json", jsonText)
	return nil
}
