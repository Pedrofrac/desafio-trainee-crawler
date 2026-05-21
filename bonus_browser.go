package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	fmt.Println("🤖 Iniciando o Chrome...")

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var bookTitle string

	// Lógica de ações do robô
	err := chromedp.Run(ctx,
		chromedp.Navigate(`https://books.toscrape.com/`), // 1. Navega para a home

		// 2. DÁ UM CLIQUE no link do primeiro livro (h3 a)
		chromedp.Click(`article.product_pod h3 a`, chromedp.ByQuery),

		// 3. ESPERA o título h1 da nova página do livro ficar visível
		chromedp.WaitVisible(`div.product_main h1`, chromedp.ByQuery),

		// 4. COPIA o texto do h1 para a nossa variável bookTitle
		chromedp.Text(`div.product_main h1`, &bookTitle, chromedp.ByQuery),
	)

	if err != nil {
		log.Fatal("Erro na automação:", err)
	}

	fmt.Println("\n🖱️ O robô clicou no primeiro livro com sucesso!")
	fmt.Printf("📖 Título do livro que o robô abriu: '%s'\n", bookTitle)
}