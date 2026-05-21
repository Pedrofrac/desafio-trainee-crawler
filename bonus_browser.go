package main
import (
	"context"
	"fmt"
	"log"
	"time"
	"github.com/chromedp/chromedp"
)
func main() {
	fmt.Println("🤖 Iniciando Chrome para ler página gerada por JavaScript...")
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var quote string
	err := chromedp.Run(ctx,
		chromedp.Navigate(`http://quotes.toscrape.com/js/`),
		chromedp.WaitVisible(`.quote`, chromedp.ByQuery),
		chromedp.Text(`.quote .text`, &quote, chromedp.ByQuery),
	)
	if err != nil {
		log.Fatal("Erro na automação:", err)
	}
	fmt.Printf("\n📖 Citação renderizada via JS extraída: %s\n", quote)
}
