# Desafio Técnico — Programa Trainee Crawler/RPA & IA

Olá! Este é o meu projeto para o Desafio Técnico. Como candidato à vaga de Trainee, meu foco aqui foi sair da zona de conforto. Desenvolvi um Web Scraper concorrente em **Go**, integrei persistência relacional (PostgreSQL), e busquei implementar **todos os bônus** exigidos pelo edital para demonstrar minha capacidade de pesquisa e aprendizado rápido.

---

## 🎁 Diferenciais e Bônus Implementados

1. **Persistência Relacional (PostgreSQL):** Gravação em lote (*Bulk Ingestion*) utilizando `ON CONFLICT DO NOTHING` para evitar duplicação de dados.
2. **Automação de Browser:** Script bônus (`bonus_browser`) utilizando `chromedp` para extrair dados gerados dinamicamente via JavaScript.
3. **Integração com IA (Gemini 2.5 Flash):** Script bônus (`bonus_ai`) que raspa um livro real do site e envia a sinopse para o Google Gemini extrair o Assunto e Sentimento via NLP.
4. **Pipeline CI/CD Otimizado:** Esteira configurada no `.gitlab-ci.yml` com etapas de Lint, Testes Automatizados (Mocks), Build de Imagem e Deploy Simulado.

---

## 🚀 Como Executar o Projeto

### 1. Rodando com Docker Compose (Scraper + Postgres)
Com o Docker aberto, execute:
```bash
docker compose up --build
```
*⚠️ **Aviso de Dívida Técnica (Linux):** No `docker-compose.yml`, fiz um bind mount em `./data`. Como a imagem Docker roda com o usuário seguro não-root (`appuser`), se você rodar isso nativamente no Linux, o daemon do Docker criará a pasta no host como `root`, o que gera erro de permissão (Permission Denied). Funciona perfeitamente no Docker Desktop (Windows/Mac).*

### 2. Rodando Localmente (Sem Docker)
Necessita de Go 1.21+ instalado.
```bash
# Baixar dependências
go mod tidy

# Rodar a suíte de testes (com mocks do banco)
go test -v ./...

# Executar o scraper principal
go run cmd/scraper/main.go
```
*O JSON e o CSV serão gerados na pasta local `./data`.*

### 3. Rodando os Scripts Bônus
* **Automação de Navegador:** `go run cmd/bonus_browser/main.go`
* **IA NLP:** (Requer configurar variável de ambiente):
```bash
export GEMINI_API_KEY="sua_chave_aqui"
go run cmd/bonus_ai/main.go
```

---

## 📊 Estrutura e Schema dos Dados

Os dados são salvos em CSV, JSON e no Banco de Dados.
**Nota de transparência:** O `image_url` raspa o caminho da miniatura da imagem (em `.jpg`), e não a página raiz do livro. Exemplo do JSON gerado:

```json
[
  {
    "title": "A Light in the Attic",
    "price": 51.77,
    "rating": 3,
    "availability": "In stock",
    "image_url": "https://books.toscrape.com/media/cache/2s/31/2s31b...jpg"
  }
]
```

---

## ⚙️ A Esteira Automática (CI/CD)

A pipeline foi construída no GitLab dividida em 4 estágios:
1. **`lint`:** Executa o `golangci-lint` para checar formatação e más práticas.
2. **`test`:** Executa `go test` acionando as validações e o Mock do Banco de Dados (`go-sqlmock`).
3. **`build`:** Usa Docker-in-Docker (`dind`) para compilar a imagem e enviá-la ao Registry do GitLab.
4. **`deploy`:** Simula um script de atualização na AWS ECS (executado apenas na branch `main`).

---

## 💡 Decisões Técnicas e Aprendizados

* **Streaming em vez de sobrecarregar RAM:** Para evitar estourar a memória caso fossem milhões de dados, o JSON é aberto e cada item é gravado imediatamente (`bufio.NewWriter`), sem agrupar fatias (*slices*) na memória.
* **Canais e Concorrência:** Usei `colly.Async(true)` para acelerar as requisições, mas centralizei a gravação jogando os dados em um canal (`booksChan`). Isso evitou que múltiplas goroutines tentassem escrever no arquivo ao mesmo tempo, prevenindo *Data Race*.

---

## 🔮 O que eu faria diferente com mais tempo? (Dívidas Técnicas)

Como estou em fase de aprendizado, tentei abraçar muitas tecnologias complexas em pouco tempo e cometi alguns erros arquiteturais que eu refatoraria no futuro:

1. **Bug no Fallback do Banco (Deadlock de Contexto):** No `main.go`, se o *Bulk Insert* falhar por tempo limite (Timeout), o meu código entra no fallback de transação individual, mas reaproveita o mesmo contexto (`dbCtx`) que já expirou. A transação nasce morta. Com mais tempo, eu geraria um novo contexto derivado do Background para essa etapa.
2. **Segurança Desabilitada no chromedp:** No bônus de Browser, estudei mitigação de DNS Rebinding/SSRF. Porém, ao injetar a flag no chromedp no meu ambiente local, a automação parou de funcionar. Por conta do prazo, deixei a flag comentada e ignorei o retorno do validador.
3. **Bloatware no Dockerfile:** Instalei as dependências pesadas do Chromium diretamente na imagem de produção para fazer o bônus funcionar, o que violou o princípio de manter a imagem base leve. Eu separaria isso em dois containers/imagens diferentes.

---

## 🤖 Transparência no Uso de Inteligência Artificial

Utilizei IA (Claude/ChatGPT) ativamente durante o desafio como ferramenta de Pair Programming, o que me ajudou muito a entender a infraestrutura, embora também tenha me ensinado a não confiar cegamente nela.

* **Prompt usado para estruturar CI/CD:** *"Quais variáveis nativas eu uso no .gitlab-ci.yml para fazer login e push num container registry, e como faço cache do Go?"*
  * **O que funcionou:** Acelerou 100% a minha sintaxe do Docker-in-Docker e configurações de stage.

* **Prompt usado para o Banco de Dados e Canais:** *"Crie uma query SQL eficiente em Go para inserir 100 livros de uma vez. Se falhar, faça um fallback usando Rollback."*
  * **O que falhou:** A IA sugeriu estruturar o *fallback* reutilizando a variável de contexto original da transação que falhou. Eu confiei na IA, implementei a lógica, e só depois percebi que isso causa um erro de *ContextDeadlineExceeded*.

* **Prompt para Segurança:** *"Como prevenir SSRF ao usar o chromedp e abrir URLs dinâmicas no Go?"*
  * **Aprendizado:** A IA me ensinou conceitos avançados (como validar se o IP é de Loopback antes de navegar), provando que é uma ótima professora, mas o código gerado trouxe bloqueios na minha máquina local que exigiram adaptação manual.
