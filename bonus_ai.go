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
	"time"
)

// Estrutura do que queremos extrair do texto usando a IA
type BookInfo struct {
	Author string `json:"author"`
	Year   int    `json:"year"`
	Pages  int    `json:"pages"`
}

// Função que lê a chave da API do arquivo local
func readAPIKey() (string, error) {
	data, err := os.ReadFile("api/key.txt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Função que chama a API do Gemini
func askGemini(apiKey string, description string) (BookInfo, error) {
	// Endpoint oficial do modelo Gemini 2.5 Flash (gratuito e muito rápido)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)

	// Criamos o prompt ensinando o que queremos
	prompt := fmt.Sprintf(`Analise o texto sobre o livro e extraia APENAS um objeto JSON com o seguinte formato:
{
  "author": "Nome do autor",
  "year": ano_de_publicacao_como_numero,
  "pages": quantidade_de_paginas_como_numero
}

Se alguma informação não estiver no texto, coloque "Desconhecido" para texto ou 0 para número. Retorne APENAS o JSON, sem markdown ou explicações.

Texto do livro: "%s"`, description)

	// Montamos o corpo da requisição exigindo que o Gemini responda em JSON
	requestBody, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json", // Força o Gemini a responder em JSON puro
		},
	})

	// Fazemos a chamada HTTP para a API do Google
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return BookInfo{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Estrutura para ler a resposta padrão do Gemini
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
	if err != nil || len(geminiResponse.Candidates) == 0 || len(geminiResponse.Candidates[0].Content.Parts) == 0 {
		return BookInfo{}, fmt.Errorf("erro ao ler resposta do Gemini: %s", string(body))
	}

	// Pega o texto JSON retornado pela IA e converte para a nossa Struct BookInfo
	jsonText := geminiResponse.Candidates[0].Content.Parts[0].Text
	var info BookInfo
	err = json.Unmarshal([]byte(jsonText), &info)
	if err != nil {
		return BookInfo{}, err
	}

	return info, nil
}

func main() {
	fmt.Println("🔑 Carregando chave da API...")
	apiKey, err := readAPIKey()
	if err != nil {
		log.Fatal("Erro ao ler chave da API em api/key.txt. Certifique-se de ter criado a pasta e o arquivo. Erro: ", err)
	}

	// 3 Casos de teste com textos completamente diferentes para provar que a IA funciona!
	testCases := []string{
		"Este clássico foi lançado em 1925 pelo aclamado F. Scott Fitzgerald e conta com 180 páginas de pura emoção.",
		"A fantástica aventura de J.R.R. Tolkien, publicada em 1937, O Hobbit, traz 310 páginas de dragões e magia.",
		"Escrevendo em 1949, George Orwell chocou o mundo com 1984, um romance distópico de 328 páginas.",
	}

	fmt.Println("🤖 Iniciando consultas ao Gemini AI...\n")

	for i, text := range testCases {
		fmt.Printf("--- Teste %d ---\nTexto Original: \"%s\"\n", i+1, text)
		
		// Espera 1 segundo entre chamadas para respeitar o limite gratuito da API
		time.Sleep(1 * time.Second)

		info, err := askGemini(apiKey, text)
		if err != nil {
			fmt.Println("❌ Erro na consulta:", err)
			continue
		}

		fmt.Println("✨ Resposta Estruturada pela IA:")
		fmt.Printf("   ✍️ Autor: %s\n", info.Author)
		fmt.Printf("   📅 Ano: %d\n", info.Year)
		fmt.Printf("   📖 Páginas: %d\n\n", info.Pages)
	}
}