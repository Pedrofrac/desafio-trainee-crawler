package main

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"os"
	"strings" // CORRIGIDO: Importação adicionada para evitar erro de compilação
	"testing"
	"context"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gocolly/colly/v2"
)

func TestCleanPrice(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
		hasError bool
	}{
		{"Preço normal", "£12.99", 12.99, false},
		{"Preço com bug de encoding", "Â£45.17", 45.17, false},
		{"String vazia", "", 0.0, true},
		{"Texto invalido", "grátis", 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cleanPrice(tt.input)
			if (err != nil) != tt.hasError {
				t.Errorf("Esperava erro: %t, mas recebeu erro: %v", tt.hasError, err)
			}
			if result != tt.expected {
				t.Errorf("Esperado %f, mas recebeu %f", tt.expected, result)
			}
		})
	}
}

func TestMapRating(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Uma estrela", "star-rating One", 1},
		{"Tres estrelas", "star-rating Three", 3},
		{"Cinco estrelas", "star-rating Five", 5},
		{"Classe invalida", "star-rating Invalido", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapRating(tt.input)
			if result != tt.expected {
				t.Errorf("Esperado %d, mas recebeu %d", tt.expected, result)
			}
		})
	}
}

func TestStartPipelineWithMock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao iniciar mock: %s", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO books").
		WithArgs("Livro de Teste", 15.50, 4, "In stock", "http://imagem.com/teste.jpg").
		WillReturnResult(sqlmock.NewResult(1, 1))

	fCSV, err := os.CreateTemp("", "test_csv")
	if err != nil {
		t.Fatalf("Erro ao criar CSV temporario: %s", err)
	}
	fJSON, err := os.CreateTemp("", "test_jsonl")
	if err != nil {
		t.Fatalf("Erro ao criar JSONL temporario: %s", err)
	}
	defer fCSV.Close()
	defer fJSON.Close()
	defer os.Remove(fCSV.Name())
	defer os.Remove(fJSON.Name())

	csvWriter := csv.NewWriter(fCSV)

	booksChan, wg := startPipeline(context.Background(), db, csvWriter, fCSV, fJSON)

	booksChan <- Book{
		Title:        "Livro de Teste",
		Price:        15.50,
		Rating:       4,
		Availability: "In stock",
		ImageURL:     "http://imagem.com/teste.jpg",
	}

	close(booksChan)
	wg.Wait()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectativas do banco nao atendidas: %s", err)
	}
}

func TestCollyParserWithMockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<article class="product_pod">
				<h3><a href="book.html" title="Livro Teste Mock">Livro Teste Mock</a></h3>
				<p class="star-rating Four"></p>
				<div class="product_price">
					<p class="price_color">£32.40</p>
					<p class="instock availability"><i class="icon-ok"></i> In stock</p>
				</div>
				<div class="image_container">
					<a href="book.html"><img src="media/test.jpg" class="thumbnail" alt="Test"></a>
				</div>
			</article>
		`))
	}))
	defer ts.Close()

	c := colly.NewCollector()
	var parsedBooks []Book

	c.OnHTML("article.product_pod", func(e *colly.HTMLElement) {
		priceCleaned, err := cleanPrice(e.ChildText(".price_color"))
		if err != nil {
			t.Errorf("Erro ao limpar preco: %v", err)
			return
		}

		book := Book{
			Title:        e.ChildAttr("h3 a", "title"),
			Price:        priceCleaned,
			Rating:       mapRating(e.ChildAttr("p.star-rating", "class")),
			Availability: strings.TrimSpace(e.ChildText(".instock.availability")),
			ImageURL:     e.Request.AbsoluteURL(e.ChildAttr(".image_container img", "src")),
		}
		parsedBooks = append(parsedBooks, book)
	})

	err := c.Visit(ts.URL)
	if err != nil {
		t.Fatalf("Erro ao visitar servidor de testes: %s", err)
	}

	if len(parsedBooks) != 1 {
		t.Fatalf("Esperava 1 livro processado, mas recebeu %d", len(parsedBooks))
	}

	res := parsedBooks[0]
	if res.Title != "Livro Teste Mock" {
		t.Errorf("Erro no Titulo: esperado 'Livro Teste Mock', mas veio '%s'", res.Title)
	}
	if res.Price != 32.40 {
		t.Errorf("Erro no Preco: esperado 32.40, mas veio %f", res.Price)
	}
	if res.Rating != 4 {
		t.Errorf("Erro no Rating: esperado 4, mas veio %d", res.Rating)
	}
	if res.Availability != "In stock" {
		t.Errorf("Erro na Disponibilidade: esperado 'In stock', mas veio '%s'", res.Availability)
	}
}
