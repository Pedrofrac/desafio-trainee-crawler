# Desafio Técnico — Programa Trainee Crawler/RPA & IA

Olá! Este é o meu projeto para o Desafio Técnico de Trainee. Eu desenvolvi um robô (scraper) usando a linguagem **Go (Golang)** que entra no site Books to Scrape, pega as informações de todos os 1.000 livros de lá, organiza tudo e salva no meu computador.

Além do projeto principal obrigatório, eu consegui entregar **os 4 bônus (diferenciais)** pedidos no desafio.

---

## 🎁 Os 4 Bônus que Entreguei

1. **`docker-compose.yml` para rodar localmente:** Criei o arquivo para rodar o projeto com apenas um comando, espelhando os arquivos para a minha máquina.
2. **Cache no Pipeline CI/CD:** Configurei o .gitlab-ci.yml para salvar em cache as ferramentas do Go, fazendo a esteira rodar muito mais rápido.
3. **Automação de Browser:** Criei um arquivo extra (`bonus_browser.go`) que usa o Chrome "invisível" para abrir o site e simular um clique real em um livro.
4. **Extração usando IA:** Criei outro arquivo (`bonus_ai.go`) que integra o Colly com o **Gemini 2.5 Flash** para capturar dados brutos e "sujos" do HTML de 3 livros reais do site e usar a IA para limpar e estruturar tudo em JSON de uma só vez (em lote).

---

## 🚀 Como Rodar o Projeto na Sua Máquina

### 1. Rodando o Robô Principal (Sem Docker)
Você precisa ter o Go instalado no seu computador.

1.1 No terminal, baixe as ferramentas do projeto:
<pre>go mod download</pre>

1.2 Rode o robô principal para extrair os 1.000 livros:
<pre>go run main.go</pre>

1.3 Rode os testes para ver se as lógicas de limpeza estão funcionando:
<pre>go test -v</pre>

### 2. Rodando o Robô Principal (Com Docker Compose)
Com o Docker Desktop aberto, rode o comando:
<pre>docker compose up --build</pre>
*Nota: O Docker vai rodar o robô de forma isolada e vai criar os arquivos de resultado direto na sua pasta local data/.*

### 3. Rodando os arquivos de Bônus (Browser e IA)
* Para ver a simulação do clique no navegador, rode:
<pre>go run bonus_browser.go</pre>
* Para ver a Inteligência Artificial limpando os dados reais do site (coloque sua chave do Gemini no arquivo `api/key.txt` primeiro):
<pre>go run bonus_ai.go</pre>

---

## 📊 Estrutura dos Dados Gerados

Quando o robô principal roda, ele cria dois arquivos na pasta data/: um JSON e um CSV. Limpei o símbolo de Libra (£) e converti as estrelas para números. Fica organizado assim:

<pre>
[
  {
    "title": "A Light in the Attic",
    "price": 51.77,
    "rating": 3,
    "availability": "In stock",
    "image_url": "https://books.toscrape.com/media/cache/2c/da/2cdad67c44b002e7ead0cc35693c0e8b.jpg"
  }
]
</pre>

---

## ⚙️ Como Funciona a Esteira Automática (Pipeline CI/CD)

No arquivo .gitlab-ci.yml, eu configurei uma esteira automática com 4 etapas. Toda vez que envio código, ela faz:

* **`lint`:** Verifica se escrevi o código de forma limpa.
* **`test`:** Roda os testes para garantir que não quebrei nada.
* **`build`:** Cria a imagem Docker e salva no registro seguro da empresa.
* **`deploy`:** Simula o envio do robô para rodar na nuvem da AWS (só funciona na branch main).

---

## 💡 Decisões Técnicas: Por que fiz assim?

* **Usei Go + Colly:** Escolhi o Go porque é rápido. O Colly lê o HTML do site sem precisar abrir um navegador real, o que deixa o processo muito mais leve. Deixei a automação de browser (Chrome) separada apenas como prova de conceito.
* **Limpeza e Estruturação:** Tive o cuidado de transformar o preço que era texto em número decimal, para que depois ele possa ser usado em cálculos no Excel ou Banco de Dados sem dar erro.
* **Segurança no Docker:** Criei um usuário restrito (não-root) chamado appuser dentro do Docker para proteger o sistema em caso de invasões.

---

## 🔮 O que eu faria diferente com mais tempo?

* **Criar uma Interface Visual (Dashboard):** Em vez de deixar os dados apenas em um arquivo de texto ou terminal, eu criaria um site simples (um frontend com HTML/React consumindo uma API em Go). Assim, pessoas não-técnicas poderiam visualizar os livros raspados de forma amigável em gráficos ou tabelas.
* **Sistema de Logs Avançado (Observabilidade):** Eu criaria uma estrutura de logs mais robusta para registrar cada passo do robô. Se o robô travasse na página 500 porque o site mudou o HTML, os logs me mostrariam exatamente em qual requisição o erro ocorreu, poupando horas de investigação.
* **Tratamento de Erros de API:** No bônus de IA, a API do Google às vezes dá erro de "muita demanda". Com mais tempo, eu programaria uma função de retry para o código esperar alguns segundos e tentar de novo sozinho.

---

## 🤖 Como usei a Inteligência Artificial durante o desafio

Neste projeto, eu utilizei a IA como um mentor particular (pair programming) para otimizar meu tempo de desenvolvimento e garantir boas práticas.

**Como ela me ajudou:**
* **Estruturação de Código e Consultas:** Usei a IA para entender como organizar o projeto em Go da melhor forma e consultar como funcionavam ferramentas específicas que eu ainda não dominava profundamente (como a sintaxe correta do Colly, a automação do Chromedp).
* **Tradução de Termos Técnicos:** A IA foi fundamental para me ajudar a "traduzir" conceitos complexos da infraestrutura. Ela me explicou de forma clara o que significava um "Pipeline CI/CD" e como funcionava um "Multi-stage build" no Docker, o que acelerou muito meu aprendizado.
* **Verificação de Erros e Atualizações:** Quando tive problemas de conflito de versões de bibliotecas ou módulos do Go desatualizados, enviei os logs de erro para a IA, que rapidamente identificou o que faltava atualizar no meu ambiente, economizando um tempo enorme de pesquisa no Google.