package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"scraper-trainee/internal/security"

	"github.com/chromedp/chromedp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("🤖 Iniciando automacao de browser...")

	var found bool
	for _, exe := range []string{"chromium", "chrome", "chromium-browser", "google-chrome"} {
		if _, err := exec.LookPath(exe); err == nil {
			found = true
			break
		}
	}
	if !found {
		slog.Warn("Executavel do Chromium/Chrome nao localizado no PATH do sistema. A execucao pode falhar.")
	}

	timeoutVal := 20 * time.Second
	if envTimeout := os.Getenv("BROWSER_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeoutVal = d
		} else {
			slog.Warn("Falha ao analisar BROWSER_TIMEOUT. Usando padrao de 20s.", "erro", err, "input", envTimeout)
		}
	}

	validationCtx, validationCancel := context.WithTimeout(context.Background(), 5*time.Second)
	//targetURL, dnsRule, err := security.ValidateTargetURL(validationCtx, os.Getenv("BROWSER_TARGET_URL"))
	targetURL, _, err := security.ValidateTargetURL(validationCtx, os.Getenv("BROWSER_TARGET_URL"))
	validationCancel()
	if err != nil {
		slog.Warn("URL de destino invalida ou nao segura. Fallback para site seguro executado.", "erro", err)
		targetURL = "http://quotes.toscrape.com/js/"
		//dnsRule = "MAP quotes.toscrape.com 104.21.68.42"
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.Flag("disable-setuid-sandbox", true),
		//chromedp.Flag("host-resolver-rules", dnsRule), 
		chromedp.UserAgent("Scraper-Bot/9.0"), 
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, timeoutVal)
	defer timeoutCancel()

	var quote string
	err = chromedp.Run(timeoutCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(`.quote`, chromedp.ByQuery),
		chromedp.Text(`.quote .text`, &quote, chromedp.ByQuery),
	)
	if err != nil {
		slog.Error("Falha critica na execucao da automacao do headless browser", "erro", err)
		return
	}
	
	slog.Info("Citação renderizada via JS extraida com sucesso!", "citacao", quote)
}
