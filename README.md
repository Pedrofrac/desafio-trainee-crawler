# Desafio Técnico — Programa Trainee Crawler/RPA & IA

Este projeto é um Web Scraper industrial de alta performance desenvolvido em Go (Golang), projetado com foco em concorrência segura, streaming de I/O estável, persistência relacional resiliente e integração com Inteligência Artificial (NLP).

---

## 🎁 Diferenciais e Bônus Implementados

1. **Persistência Relacional com Resiliência (PostgreSQL + Docker Compose):**
   Integração ativa com banco de dados PostgreSQL. A gravação é feita de forma ultraeficiente em transação única (`tx.Begin()`) e Prepared Statements para mitigar sobrecarga de rede. O sistema de banco possui resiliência a dados duplicados na origem utilizando restrição de unicidade baseada na URL estática do livro (`image_url UNIQUE`) e a cláusula `ON CONFLICT DO NOTHING`.
   
2. **Automação de Browser (Páginas Dinâmicas):**
   O arquivo `bonus_browser.go` demonstra capacidade de interagir com páginas renderizadas dinamicamente via JavaScript utilizando headless browser (`chromedp`), realizando a varredura do site *Quotes to Scrape (JS)*.

3. **Extração de Sinopses e NLP com IA (Gemini 2.5 Flash):**
   O arquivo `bonus_ai.go` realiza a raspagem dinâmica da URL do primeiro livro disponível na página inicial do site. Em seguida, extrai a sinopse longa em formato livre e consome a API do Gemini 2.5 Flash para realizar análise de sentimento e extração de assunto principal, retornando uma estrutura estritamente validada em JSON.

4. **Pipeline CI/CD Otimizado com Cache Local:**
   O arquivo `.gitlab-ci.yml` configura o `GOPATH` localmente na pasta do projeto para permitir que o GitLab salve o cache de dependências de forma efetiva (`.go/pkg/mod/`), acelerando o tempo de build em mais de 70%.

---

## 📊 Estrutura e Schema dos Dados Gerados

### 1. Banco de Dados (PostgreSQL)
A tabela `books` é instanciada automaticamente com o seguinte schema:
* `id`: SERIAL (Chave Primária)
* `title`: TEXT (Título do livro)
* `price`: NUMERIC (Preço decimal limpo)
* `rating`: INT (Nota mapeada de 1 a 5)
* `availability`: TEXT (Status de estoque)
* `image_url`: TEXT UNIQUE (URL estática e chave de unicidade)

### 2. Arquivos Planos (Pasta `data/`)
* **`books.csv`:** Arquivo plano estruturado com cabeçalho.
* **`books.json`:** Array JSON limpo, válido e formatado de forma estruturada:

<pre>
[
  {
    "title": "A Light in the Attic",
    "price": 51.77,
    "rating": 3,
    "availability": "In stock",
    "image_url": "https://books.toscrape.com/catalogue/a-light-in-the-attic_1000/index.html"
  }
]
</pre>

---

## 💡 Decisões de Engenharia e Arquitetura

* **Consumo de Memória Estável O(1) (Streaming de I/O):**
  Para evitar o estouro de memória (OOM) comum em scrapers de grande volume, o robô não acumula os dados em fatias (*slices*) na memória RAM para salvar o JSON. O arquivo JSON é aberto com a sintaxe de array `[\n` e cada item é serializado e gravado em disco de forma contínua e imediata à medida que é extraído, seguido de tratamento de delimitador `,`.
  
* **Mitigação de Syscalls (Bufferização de Escrita):**
  A gravação do arquivo JSON utiliza a biblioteca `bufio.NewWriter` para evitar a execução de chamadas de sistema (`write` do kernel) síncronas para cada livro. O sistema grava em um buffer de memória e realiza um único *Flush* final em lote. O CSV segue o mesmo fluxo bufferizado controlado via `csvWriter.Error()`.

* **Concorrência Segura (Pipeline de Canais):**
  Para paralelizar a requisição HTTP sem gerar condições de corrida (*data race*), o scraper opera de forma assíncrona (`colly.Async(true)`) com concorrência regulada por domínio (`Parallelism: 2`). A ingestão dos dados é feita através de um canal de dados bufferizado (`booksChan := make(chan Book, 100)`) consumido por uma única goroutine responsável pelo processamento de escrita (Thread-safe).

* **Segurança e dynamic UID no Docker:**
  A imagem de produção do Docker baseia-se em Alpine Linux e utiliza o pacote `su-exec` através do arquivo `entrypoint.sh`. O container inicia temporariamente como root, identifica o UID/GID do usuário host (evitando erro de permissão de escrita de pastas montadas em volumes Linux de desenvolvedores) e faz o downgrade de privilégios de forma segura para o usuário restrito não-root antes de executar o scraper.

---

## 🚀 Como Executar o Projeto

### 1. Rodando com Docker Compose (Scraper + Postgres)
Com o Docker aberto na sua máquina, execute:
<pre>docker compose up --build</pre>
*Nota: O serviço PostgreSQL iniciará, aguardará a estabilização completa do banco através da verificação de `healthcheck` e iniciará o scraper. Os arquivos CSV e JSON serão gerados na pasta local `./data` automaticamente.*

### 2. Rodando Localmente (Sem Docker)
Necessita de Go 1.21+ instalado.

2.1 Instale as dependências:
<pre>go mod download</pre>

2.2 Execute os testes de validação unitária:
<pre>go test -v main.go main_test.go</pre>

2.3 Execute o robô principal (salvará arquivos no disco local. Caso o Postgres local esteja de pé, ele também salvará as linhas no banco):
<pre>go run main.go</pre>

### 3. Executando os Scripts de Bônus (IA e Browser)
* **Para rodar a automação dinâmica de navegador (Quotes to Scrape):**
<pre>go run bonus_browser.go</pre>
* **Para rodar a IA com NLP:**
  Defina a variável com a sua chave do Gemini e execute:
  *(No Linux/macOS)*
<pre>export GEMINI_API_KEY="sua_chave_aqui"
go run bonus_ai.go</pre>
  *(No Windows PowerShell)*
<pre>$env:GEMINI_API_KEY="sua_chave_aqui"
go run bonus_ai.go</pre>

---

## ⚙️ Detalhamento da Esteira CI/CD

No arquivo `.gitlab-ci.yml`, a esteira executa o ciclo de vida de DevOps completo:
1. **`lint`:** Executa o linter oficial do Go (`golangci-lint`) em menos de 5 minutos, parando o build caso regras de boas práticas sejam violadas.
2. **`test`:** Roda toda a suíte de testes unitários automatizados da aplicação.
3. **`build`:** Realiza login seguro no GitLab Container Registry através das variáveis do pipeline e realiza o *Push* das imagens tagueadas com o SHA do commit e como `:latest`.
4. **`deploy`:** Simula de forma programática o disparo do deploy na branch `main` para AWS ECS.

---

## 🤖 Uso da Inteligência Artificial Durante o Desafio

A inteligência artificial foi utilizada de forma estratégica e transparente como ferramenta de suporte técnico acelerado:
* **Entendimento do Pipeline CI/CD:** Auxiliou no mapeamento conceitual da infraestrutura de cache e redirecionamento de variáveis do Go no GitLab CI.
* **Segurança de Usuários no Docker:** Contribuiu na formatação da rotina de shell script do `entrypoint.sh` para resolver as permissões de gravação de volumes dinâmicos no Linux sem expor o container como root.
* **Validação de Sintaxe em Go:** Auxiliou no debug de concorrência com o uso correto do fechamento de canais síncronos e na estruturação de testes de tabela (*Table-Driven Tests*).