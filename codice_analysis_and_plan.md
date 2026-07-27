# Códice — Plano Mestre de Refatoração v3

> **Documento auto-contido**. Este plano pode ser lido por qualquer agente ou desenvolvedor sem acesso à conversa original. Contém todo o contexto necessário.

---

## 0. Resumo Executivo

### O que é o Códice
**Códice** é uma plataforma self-hosted para gerenciar obras de leitura — livros, quadrinhos, mangás, artigos, etc. — nos formatos PDF, EPUB, MOBI, CBZ, CBR, MD, TXT e futuramente audiobooks. O projeto usa **Go** no backend, **Python** nos workers assíncronos, e **React** (Vite) no frontend. Licença: **AGPL-3.0**.

- **Repositório**: `/Users/bianco/repos/codice`
- **Backend Go**: `/Users/bianco/repos/codice/backend` (Chi router, PostgreSQL, Redis, JWT)
- **Worker Python**: `/Users/bianco/repos/codice/worker` (PyMuPDF, Redis Streams)
- **Frontend React**: `/Users/bianco/repos/codice/frontend` (Vite, TanStack Query, Zustand)

### O que precisa ser feito
Duas grandes áreas estão quebradas/bagunçadas e precisam de refatoração completa:

1. **Leitores de Arquivos** — O MangaViewer carrega CBZ inteiro no browser (crashes), viewers não têm error handling, URLs hardcoded quebram deploy
2. **Gerenciador de Metadados** — O scraper atual nunca funcionou direito, extração genérica via PyMuPDF perde metadados ricos, sem ComicInfo.xml

Além disso, a análise revelou problemas graves de **segurança**, **configuração (.env)** e **performance** que precisam ser resolvidos.

### Projetos de Inspiração

Dois projetos open-source foram analisados em profundidade para extrair padrões arquiteturais. **Nenhum código foi ou será copiado** — apenas a abordagem/arquitetura é usada como referência.

#### 📚 Calibre-Web (GPL-3.0)
- **Repo**: https://github.com/janeczku/calibre-web
- **Pontos fortes usados**: Arquitetura de metadata providers plugáveis (Google Books, Amazon, ComicVine, Douban), extração multi-formato com libs especializadas (`lxml` para EPUB OPF, `pypdf` para PDF, `comicapi` para ComicInfo.xml, `mutagen` para audiobooks), UI de edição de metadados rica, sistema de identificadores (ISBN, Goodreads, ASIN)
- **Viewers**: epub.js, pdf.js, kthoom para comics, HTML5 audio player
- **Schema**: Dual-database (metadata.db do Calibre + app.db próprio)

#### 📖 Komga (MIT)
- **Repo**: https://github.com/gotson/komga
- **Pontos fortes usados**: Server-side page streaming (nunca envia o ZIP inteiro pro browser), abstração `MediaContainerExtractor` unificada, sistema de metadata locks (campo editado manualmente não é sobrescrito por rescan), pipeline de media analysis com status lifecycle (UNKNOWN→READY→ERROR→OUTDATED), modos de leitura (LTR/RTL/Webtoon/Double-page), OPDS 1.2/2.0, conversão de imagens on-the-fly
- **Schema**: SQLite com jOOQ, entidades BOOK→MEDIA→MEDIA_PAGE, SERIES_METADATA com lock columns

> [!NOTE]
> A licença AGPL-3.0 do Códice é compatível com GPL-3.0 (Calibre-Web) e MIT (Komga). Referências serão documentadas no README.md.

---

## 1. Auditoria do .env e Docker Compose

### 1.1 — Estado Atual do `.env.example`

```
# .env.example (ATUAL — 17 linhas, incompleto)
POSTGRES_USER=codice_user
POSTGRES_PASSWORD=codice_secret
POSTGRES_DB=codice_db
POSTGRES_PORT=5432
REDIS_PORT=6379
DATABASE_URL=postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable
REDIS_URL=redis://localhost:6379/0
JWT_SECRET=super_secret_key_change_in_production
```

### 1.2 — Problemas Encontrados

#### 🔴 Variáveis Faltantes

| Variável | Quem precisa | Onde é usada | Impacto |
|:---------|:------------|:-------------|:--------|
| `CODICE_STORAGE_PATH` | Backend + Worker | [main.go L121](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L121), [processor.py L12](file:///Users/bianco/repos/codice/worker/processor.py#L12), [scraper.py L12](file:///Users/bianco/repos/codice/worker/scraper.py#L12) | Sem isso, backend usa `./uploads` e worker usa `../backend/uploads` — paths diferentes! |
| `CORS_ALLOWED_ORIGINS` | Backend | [main.go L89](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L89) | Fallback `http://localhost:5173` quebra em produção |
| `PORT` | Backend | [main.go L143](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L143) | Fallback `8080`, mas deveria ser documentado |
| `VITE_API_URL` | Frontend | [api.js L4](file:///Users/bianco/repos/codice/frontend/src/lib/api.js#L4) | Fallback `http://localhost:8080`, mas **variáveis Vite são build-time** — não funcionam em runtime Docker |
| `APP_ENV` | Todos | Nenhum (mas deveria) | Sem flag dev/prod, auth middleware permite acesso admin sem token |

#### 🔴 Path Relativo Quebrado no `.env` Loading

**Backend** ([config.go L12](file:///Users/bianco/repos/codice/backend/internal/config/config.go#L12)):
```go
godotenv.Load("../.env")  // Relativo ao CWD, não ao executável
```
- ✅ Funciona em dev se rodar `go run` de `/backend/cmd/api/`
- ❌ Quebra se rodar de qualquer outro diretório
- ❌ No Docker, o `environment:` block injeta vars direto → `.env` nunca é carregado (funciona por coincidência)

**Worker** ([main.py L17](file:///Users/bianco/repos/codice/worker/main.py#L17)):
```python
load_dotenv(dotenv_path="../.env")  # Mesmo problema
```
- ✅ Funciona em dev se rodar de `/worker/`
- ❌ No Docker, mesma situação — funciona por coincidência via `environment:` block

#### 🔴 Docker Compose: Credenciais Duplicadas e Hardcoded

O [docker-compose.yml](file:///Users/bianco/repos/codice/docker-compose.yml) hardcoda `codice_user:codice_secret` em **4 lugares** (postgres, backend, worker duplicam `DATABASE_URL`). Deveria usar `${VARIABLE}` referenciando o `.env`.

Também usa `version: '3.8'` que é deprecated — Docker Compose moderno ignora o campo `version`.

#### 🟡 Frontend: `VITE_API_URL` Nunca Funciona em Docker

Variáveis `VITE_*` são injetadas em **build time** pelo Vite (são substituídas no bundle JS). No Docker:
1. O `Dockerfile` do frontend faz `npm run build` 
2. O bundle resultante tem `import.meta.env.VITE_API_URL` substituído pelo valor que existia **durante o build**
3. Se não estava definido no build → fica `undefined` → fallback `http://localhost:8080`
4. Em produção com domínio customizado, **nada funciona**

**Solução**: Usar runtime config injection (ex: `window.__CODICE_CONFIG__` via script no `index.html` servido pelo Nginx, ou fazer o frontend usar paths relativos + proxy reverso).

### 1.3 — `.env.example` Proposto

```bash
# ============================================================
# Códice — Environment Configuration
# Copy this file to .env and edit values for your environment
# ============================================================

# ---------- Application ----------
APP_ENV=development                    # 'development' or 'production'
# In production: disables auth dev bypass, enforces JWT, enables security headers

# ---------- PostgreSQL ----------
POSTGRES_USER=codice_user
POSTGRES_PASSWORD=change_me_in_production
POSTGRES_DB=codice_db
POSTGRES_HOST=localhost                 # Use 'postgres' for Docker
POSTGRES_PORT=5432

# Connection URL (assembled from above, or set directly)
DATABASE_URL=postgres://codice_user:change_me_in_production@localhost:5432/codice_db?sslmode=disable

# ---------- Redis ----------
REDIS_HOST=localhost                    # Use 'redis' for Docker
REDIS_PORT=6379
REDIS_URL=redis://localhost:6379/0

# ---------- Storage ----------
CODICE_STORAGE_PATH=./uploads           # Absolute path in production (e.g., /data/codice)

# ---------- Backend Server ----------
PORT=8080
CORS_ALLOWED_ORIGINS=http://localhost:5173    # Comma-separated for multiple origins

# ---------- Authentication ----------
JWT_SECRET=CHANGE_THIS_TO_A_RANDOM_64_CHAR_STRING
JWT_EXPIRATION_HOURS=168                # 7 days default. Reduce for production.

# ---------- Frontend (Build-time) ----------
VITE_API_URL=http://localhost:8080      # Set before 'npm run build'. Empty = relative URLs.

# ---------- Metadata Providers (Optional) ----------
# GOOGLE_BOOKS_API_KEY=                 # Optional: increases rate limit
# COMICVINE_API_KEY=                    # Required for ComicVine provider
# ANILIST_API_KEY=                      # Optional: AniList is public API
```

### 1.4 — Docker Compose Proposto

```yaml
services:
  postgres:
    image: postgres:15-alpine
    container_name: codice_db
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-codice_user}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-codice_secret}
      POSTGRES_DB: ${POSTGRES_DB:-codice_db}
    volumes:
      - codice_pgdata:/var/lib/postgresql/data
    networks:
      - codice_network
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    container_name: codice_redis
    networks:
      - codice_network
    restart: unless-stopped

  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    container_name: codice_backend
    environment:
      APP_ENV: ${APP_ENV:-production}
      DATABASE_URL: postgres://${POSTGRES_USER:-codice_user}:${POSTGRES_PASSWORD:-codice_secret}@postgres:5432/${POSTGRES_DB:-codice_db}?sslmode=disable
      REDIS_URL: redis://redis:6379/0
      CODICE_STORAGE_PATH: /app/storage
      JWT_SECRET: ${JWT_SECRET:?JWT_SECRET must be set}
      CORS_ALLOWED_ORIGINS: ${CORS_ALLOWED_ORIGINS:-http://localhost:5173}
      PORT: "8080"
    ports:
      - "8080:8080"
    volumes:
      - ./uploads:/app/storage
    depends_on:
      - postgres
      - redis
    networks:
      - codice_network
    restart: unless-stopped

  worker:
    build:
      context: ./worker
      dockerfile: Dockerfile
    container_name: codice_worker
    environment:
      APP_ENV: ${APP_ENV:-production}
      DATABASE_URL: postgres://${POSTGRES_USER:-codice_user}:${POSTGRES_PASSWORD:-codice_secret}@postgres:5432/${POSTGRES_DB:-codice_db}?sslmode=disable
      REDIS_URL: redis://redis:6379/0
      CODICE_STORAGE_PATH: /app/storage
    volumes:
      - ./uploads:/app/storage
    depends_on:
      - postgres
      - redis
    networks:
      - codice_network
    restart: unless-stopped

  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
      args:
        VITE_API_URL: ${VITE_API_URL:-}
    container_name: codice_frontend
    ports:
      - "5173:80"
    depends_on:
      - backend
    networks:
      - codice_network
    restart: unless-stopped

networks:
  codice_network:
    driver: bridge

volumes:
  codice_pgdata:
```

**Mudanças-chave**:
- Todas as credenciais vêm de variáveis com `${VAR:-default}`
- `JWT_SECRET` usa `${VAR:?error}` — **falha** se não estiver definido (obrigatório)
- `version:` removido (deprecated)
- Frontend recebe `VITE_API_URL` como build arg
- `APP_ENV` adicionado para backend e worker

---

## 2. Auditoria de Segurança

### 🔴 Severidade Crítica

#### SEC-01: Auth Middleware Permite Admin Sem Token
**Arquivo**: [auth.go (middleware)](file:///Users/bianco/repos/codice/backend/internal/middleware/auth.go#L35-L41)
```go
if authHeader == "" {
    // Development fallback: admin access without any token!
    ctx := context.WithValue(r.Context(), UserIDKey, DefaultDevUserID)
    ctx = context.WithValue(ctx, UserRoleKey, "admin")
    next.ServeHTTP(w, r.WithContext(ctx))
    return
}
```
**Impacto**: Qualquer request sem header Authorization recebe acesso admin completo. Em produção, isso significa que **qualquer pessoa** pode editar, deletar e fazer upload sem autenticação.

**Fix**: Condicionar ao `APP_ENV`:
```go
if authHeader == "" {
    if os.Getenv("APP_ENV") == "development" {
        // Dev fallback...
    } else {
        http.Error(w, "Authorization required", http.StatusUnauthorized)
        return
    }
}
```

#### SEC-02: Arquivos Servidos Sem Autenticação
**Arquivo**: [main.go L132-140](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L132-L140)
```go
r.Get("/covers/*", ...)   // SEM AuthMiddleware
r.Get("/files/*", ...)    // SEM AuthMiddleware
```
**Impacto**: Qualquer pessoa pode baixar qualquer livro/cover se souber o path. Sem autenticação.

**Fix**: Proteger com auth via query param token (vide Sprint 0).

#### SEC-03: Endpoint `/works` (listagem) Sem Auth
**Arquivo**: [main.go L111](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L111)
```go
r.Get("/works", libHandler.GetWorks)  // SEM AuthMiddleware!
```
**Impacto**: Toda a biblioteca é acessível publicamente.

#### SEC-04: WebSocket Sem Auth + Origin Wildcard
**Arquivo**: [ws.go L14-16](file:///Users/bianco/repos/codice/backend/internal/handlers/ws.go#L14-L16), [main.go L118](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L118)
```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },  // Aceita qualquer origin
}
r.Get("/ws", wsHandler.HandleWS)  // SEM AuthMiddleware
```

#### SEC-05: JWT Secret Hardcoded em Fallback
**Arquivo**: [auth.go (middleware) L14](file:///Users/bianco/repos/codice/backend/internal/middleware/auth.go#L14) e [auth.go (handler) L30](file:///Users/bianco/repos/codice/backend/internal/handlers/auth.go#L30)
```go
secret = "default_codice_jwt_secret_key_change_me"
```
**Impacto**: Se `JWT_SECRET` não estiver definido, todos os tokens usam um secret público. Qualquer pessoa pode forjar tokens JWT válidos.

**Fix**: Panic/fatal se `JWT_SECRET` não estiver definido em produção.

#### SEC-06: Função `getJWTSecret()` Duplicada
Existe em **dois arquivos** separados: `middleware/auth.go` e `handlers/auth.go`. Se alguém alterar em um e esquecer do outro, a validação e a geração de tokens usam secrets diferentes → **todos os tokens ficam inválidos**.

### 🟡 Severidade Alta

#### SEC-07: `/auth/register` Público Sem Controle
**Arquivo**: [main.go L107](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L107)
```go
r.Post("/auth/register", authHandler.Register)  // Qualquer pessoa cria conta
```
**Recomendação**: Permitir registro apenas se habilitado nas settings, ou exigir convite/aprovação do admin.

#### SEC-08: JWT Expira em 7 Dias Sem Refresh Token
**Arquivo**: [auth.go L102](file:///Users/bianco/repos/codice/backend/internal/handlers/auth.go#L102)
```go
expirationTime := time.Now().Add(7 * 24 * time.Hour)
```
**Recomendação**: Token de acesso curto (1h) + refresh token (7d), ou tornar configurável via env.

#### SEC-09: Sem Rate Limiting nos Endpoints de Auth
Nenhum rate limit em `/auth/login`, `/auth/register`, `/auth/setup`. Brute force trivial.

**Recomendação**: Usar middleware de rate limiting (ex: `go-chi/httprate`).

#### SEC-10: Sem Validação de Algoritmo JWT
**Arquivo**: [auth.go (middleware) L45-47](file:///Users/bianco/repos/codice/backend/internal/middleware/auth.go#L45-L47)
```go
token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
    return getJWTSecret(), nil  // Não valida se o algoritmo é HS256!
})
```
**Impacto**: Vulnerável a algorithm confusion attacks (ex: trocar para `none`).

**Fix**:
```go
if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
    return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
}
```

### 🟢 Severidade Média/Baixa

#### SEC-11: Cover Fallback Usa Serviço Externo
**Arquivo**: [library.go L67](file:///Users/bianco/repos/codice/backend/internal/handlers/library.go#L67)
```go
work.CoverURL = "https://via.placeholder.com/300x450/..."  // Serviço externo
```
Uma plataforma self-hosted não deveria depender de serviço externo para placeholder. Usar SVG inline ou imagem local.

#### SEC-12: CORS Aceita Apenas Uma Origin
**Arquivo**: [main.go L94-95](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L94-L95)
```go
AllowedOrigins: []string{allowedOrigin}  // String única, não split por vírgula
```
Se `CORS_ALLOWED_ORIGINS` for `http://localhost:5173,https://codice.example.com`, passa como uma string só.

#### SEC-13: Erros do Delete Não São Verificados
**Arquivo**: [library.go L309-311](file:///Users/bianco/repos/codice/backend/internal/handlers/library.go#L309-L311)
```go
tx.Exec("DELETE FROM work_tags WHERE work_id = $1", id)      // err ignorado
tx.Exec("DELETE FROM user_progress WHERE work_id = $1", id)  // err ignorado
tx.Exec("DELETE FROM editions WHERE work_id = $1", id)       // err ignorado
```

---

## 3. Auditoria de Performance

#### PERF-01: Sem Paginação na Listagem
**Arquivo**: [library.go L33-48](file:///Users/bianco/repos/codice/backend/internal/handlers/library.go#L33-L48)
`GetWorks` retorna **todas** as obras sem `LIMIT/OFFSET`. Com 10.000+ livros, response fica enorme.

#### PERF-02: Sem Connection Pooling Configurado
**Arquivo**: [main.go L33](file:///Users/bianco/repos/codice/backend/cmd/api/main.go#L33)
```go
db, err := sql.Open("postgres", dbURL)  // Pool defaults (sem MaxOpenConns, MaxIdleConns)
```
**Fix**: `db.SetMaxOpenConns(25); db.SetMaxIdleConns(5); db.SetConnMaxLifetime(5 * time.Minute)`

#### PERF-03: Sem Compressão HTTP (gzip)
Backend não usa middleware de compressão. Responses JSON e assets são enviados sem comprimir.

**Fix**: Adicionar `middleware.Compress(5)` do chi.

#### PERF-04: Sem Cache Headers em Assets Estáticos
Covers e arquivos são servidos sem `Cache-Control`, `ETag`, ou `Last-Modified`. Browser refaz download a cada acesso.

#### PERF-05: Worker Single Consumer
**Arquivo**: [main.py L33](file:///Users/bianco/repos/codice/worker/main.py#L33)
```python
CONSUMER_NAME = 'worker_1'  # Hardcoded, sem scaling horizontal
```

#### PERF-06: Sem Índices de Busca Otimizados
`library_search_index` (materialized view com tsvector) existe no schema mas nunca é populada ou usada. `GetWorks` faz busca sequencial no frontend.

#### PERF-07: Cover Images Não Otimizadas
Covers extraídas de PDFs via `fitz.Matrix(2, 2)` geram imagens grandes. Sem resize para thumbnails.

#### PERF-08: Frontend Carrega Todas as Works Para Filtrar Client-Side
[BookGrid.jsx](file:///Users/bianco/repos/codice/frontend/src/features/library/components/BookGrid.jsx) faz `useWorks()` (todas) e filtra com `useMemo` no client. Deveria ter search/filter server-side.

---

## 4. Diagnóstico do Frontend (Carregamento de Arquivos)

> [!CAUTION]
> Este é o "outro grande problema" mencionado — como o frontend carrega os arquivos, que "muuuitas vezes dá problema".

### Problema Raiz: Chain of Failures

```mermaid
flowchart TD
    A["Backend retorna fileUrl absoluto<br>http://localhost:8080/files/..."] --> B{"Browser tenta carregar"}
    B -->|Deploy não-localhost| C["❌ URL errada → CORS error ou 404"]
    B -->|localhost| D{"Viewer tenta usar URL"}
    D -->|PdfViewer| E["Document file={fileUrl}<br>Sem auth header → funciona<br>pois /files/ não tem auth"]
    D -->|EpubViewer| F["ReactReader url={fileUrl}<br>Sem auth header → funciona<br>mas sem error handling"]
    D -->|MangaViewer| G["fetch(fileUrl) → blob<br>Sem auth → baixa CBZ inteiro<br>→ JSZip no browser → 💥 crash"]
    
    E --> H["Sem onLoadError<br>Se falhar → loading infinito"]
    F --> I["Sem error boundary<br>Se EPUB corrompido → tela branca"]
    G --> J["Memory leak se unmount<br>durante decompressão"]
```

### 4 Falhas Concretas

1. **URLs absolutas hardcoded** → Backend [library.go L141](file:///Users/bianco/repos/codice/backend/internal/handlers/library.go#L141) retorna `http://localhost:8080/files/...`. Fora de localhost, nada carrega.

2. **Nenhum viewer envia JWT** → `PdfViewer` passa URL direto para `<Document file={}>`, `EpubViewer` para `<ReactReader url={}>`, `MangaViewer` usa `fetch()` sem header Auth. Nenhum usa a instância `api` (axios com interceptor).

3. **MangaViewer baixa CBZ inteiro** → [MangaViewer.jsx](file:///Users/bianco/repos/codice/frontend/src/features/reader/components/viewers/MangaViewer.jsx) faz `fetch(fileUrl)` → `response.blob()` → `JSZip.loadAsync(blob)` → cria Object URLs para **todas** as páginas. Arquivos de 200MB+ crasham o browser.

4. **Parsing de extensão quebrado** → [Reader.jsx](file:///Users/bianco/repos/codice/frontend/src/features/reader/components/Reader.jsx): `book.fileUrl.split('.').pop()` retorna `"pdf?v=1"` se URL tiver query params.

---

## 5. Decisões de Design — Resolvidas

| # | Decisão | Resultado |
|:--|:--------|:----------|
| 1 | **Progresso de leitura** | Formato estruturado: `progress_type` (page/cfi/time) + `progress_percent` (REAL). EPUB usa CFI (independente de viewport/fonte). Page-based usa número da página. |
| 2 | **Thumbnails** | Gerar durante análise no Python worker (como Komga). Cache em disco. |
| 3 | **OPDS** | Sim. OPDS 1.2 para compatibilidade com apps mobile. |
| 4 | **PDF serving** | Manter `pdf.js` client-side + HTTP Range Requests no backend Go. Thumbnails server-side no worker. |
| 5 | **Media analysis pipeline** | Python worker via Redis (assíncrono). Qualidade > velocidade. |
| 6 | **Sistema de metadados** | **Apagar tudo** (`scraper.py`, `processor.py`) e reescrever do zero inspirado no Calibre-Web. |

---

## 6. Plano de Execução — 7 Sprints

### Sprint 0: Infraestrutura, Segurança e .env (pré-requisito)

> [!IMPORTANT]
> Tudo aqui precisa ser feito **antes** de qualquer feature nova. São bugs que afetam toda a plataforma.

| # | Tarefa | Arquivos | Severidade |
|:--|:-------|:---------|:-----------|
| 0.1 | Reescrever `.env.example` completo (seção 1.3 acima) | `.env.example` | 🔴 |
| 0.2 | Docker Compose usar variáveis `${VAR}` em vez de hardcode | `docker-compose.yml` | 🔴 |
| 0.3 | Backend: retornar paths relativos em vez de URLs absolutas | `library.go` L141, `processor.py` L125/144, `scraper.py` L92 | 🔴 |
| 0.4 | **SEC-01**: Auth middleware respeitar `APP_ENV` — rejeitar requests sem token em produção | `middleware/auth.go` | 🔴 |
| 0.5 | **SEC-02/03**: Proteger `/files/*`, `/covers/*`, `/works` com auth (query param token para assets) | `main.go` | 🔴 |
| 0.6 | **SEC-04**: WebSocket requer auth + validar origin | `ws.go`, `main.go` | 🔴 |
| 0.7 | **SEC-05/06**: Unificar `getJWTSecret()` em um único lugar, panic se vazio em prod | `middleware/auth.go`, `handlers/auth.go` | 🔴 |
| 0.8 | **SEC-10**: Validar algoritmo JWT no parse | `middleware/auth.go` | 🟡 |
| 0.9 | **SEC-07**: Registro controlado (settings ou invite-only) | `handlers/auth.go`, `main.go` | 🟡 |
| 0.10 | **SEC-09**: Rate limiting em endpoints de auth | `main.go` (adicionar dep `go-chi/httprate`) | 🟡 |
| 0.11 | Frontend: WebSocket dinâmico (`window.location`) | `App.jsx` L46 | 🔴 |
| 0.12 | Frontend: Helper `authenticatedUrl(path)` para assets | `lib/api.js` (novo) | 🔴 |
| 0.13 | Fix parsing de extensão no Reader | `Reader.jsx` — usar `work.format` do backend | 🟡 |
| 0.14 | **PERF-02**: Connection pooling PostgreSQL | `main.go` | 🟡 |
| 0.15 | **PERF-03**: Middleware de compressão gzip | `main.go` | 🟡 |
| 0.16 | **PERF-04**: Cache headers em assets estáticos | `main.go` (custom handler com `Cache-Control`) | 🟢 |
| 0.17 | **SEC-11**: Placeholder cover local (SVG/PNG) em vez de via.placeholder.com | `library.go` L67, L129 | 🟢 |
| 0.18 | **SEC-13**: Verificar erros do DELETE em cascata | `library.go` L309-311 | 🟢 |
| 0.19 | Config loader resiliente (não depender de path relativo `../.env`) | `config.go`, `main.py` | 🟡 |

---

### Sprint 1: Server-Side Page Streaming

Substituir o carregamento client-side do CBZ por streaming server-side página por página.

**Novos Endpoints Go**:
```
GET /api/v1/works/{id}/pages                → JSON: lista de páginas com metadata
GET /api/v1/works/{id}/pages/{page}         → Binary: imagem streaming direto do ZIP
GET /api/v1/works/{id}/pages/{page}/thumbnail → Binary: thumbnail reduzida
```

**Implementação Go** (CBZ): `archive/zip` nativo → stream da entry diretamente → `http.ServeContent` com Range support.

**Reescrever MangaViewer.jsx** → Remover JSZip, usar `<img src={authenticatedUrl(...)} />` por página, preload ±2 adjacentes.

**Novos modos de leitura**: LTR, RTL (mangá), Webtoon (scroll vertical), Double-page spread.

**HTTP Range Requests**: Habilitar em `/files/*` para que `pdf.js` faça streaming incremental de PDFs grandes.

**Error handling**: `onLoadError` em todos os viewers, loading skeletons, timeout 30s com fallback.

---

### Sprint 2: Sistema de Metadados — Reescrita Total

> [!CAUTION]
> `scraper.py` e `processor.py` atuais serão **deletados**. Novo sistema construído do zero.

**Nova estrutura**:
```
worker/
├── extractors/
│   ├── base.py              # ABC → ExtractedMetadata dataclass
│   ├── epub_extractor.py    # lxml + zipfile → OPF Dublin Core
│   ├── pdf_extractor.py     # pypdf → Document Info Dict
│   ├── cbz_extractor.py     # zipfile → ComicInfo.xml
│   ├── cbr_extractor.py     # rarfile → ComicInfo.xml
│   ├── mobi_extractor.py    # EXTH binary parser
│   ├── txt_extractor.py     # Filename-based
│   └── audio_extractor.py   # mutagen → ID3/MP4
├── analyzer.py              # Media analysis pipeline (status lifecycle)
├── main.py                  # Redis consumer (refatorado)
└── requirements.txt
```

**Migrations**: `work_identifiers`, `media_pages`, metadata lock columns, `media_status`, `reading_direction`, `format`, `progress_type`, `progress_percent`.

---

### Sprint 3: Metadata Providers — Reescrita Total

**Nova estrutura**:
```
worker/providers/
├── base.py              # ABC → MetaRecord dataclass
├── google_books.py      # Google Books API
├── openlibrary.py       # OpenLibrary API
├── comicvine.py         # ComicVine API (comics)
├── mangaupdates.py      # MangaUpdates API (mangá)
├── anilist.py           # AniList GraphQL (mangá/LN)
└── registry.py          # Seleção por formato + prioridade
```

Seleção inteligente: CBZ/CBR → ComicVine/MangaUpdates primeiro; EPUB/PDF → Google Books/OpenLibrary primeiro.

Cover download + cache local. Respeitar `cover_lock`.

---

### Sprint 4: Frontend Melhorado

- Error boundaries em todos os viewers
- UI de edição de metadados expandida (mais campos, locks, search providers)
- **PERF-01/08**: Paginação server-side + search server-side (usar `library_search_index`)
- Shelf/Collection system

---

### Sprint 5: Novos Formatos

| Formato | Worker | Backend | Frontend |
|:--------|:-------|:--------|:---------|
| CBR | `rarfile` | Endpoints de páginas (= CBZ) | Mesmo ComicViewer |
| TXT | Título do filename | Servir via Range | Custom reader |
| MD | Título do filename | Servir texto | `react-markdown` |
| MOBI/AZW | EXTH parser | Conversão → EPUB | EpubViewer |
| Audiobook | `mutagen` | Audio stream Range | HTML5 `<audio>` |

---

### Sprint 6: OPDS

OPDS 1.2 Catalog no backend Go:
```
GET /opds/v1.2/catalog     → Root (navigation feed, Atom XML)
GET /opds/v1.2/search?q=   → Search (acquisition feed)
GET /opds/v1.2/recent      → Recently added
GET /opds/v1.2/series/{id} → Series listing
```
Auth: Basic Auth (compat apps mobile) + JWT. Compat: Panels, Chunky, Mihon, KOReader, Moon+ Reader.

---

## 7. Referências para o README.md

```markdown
## Acknowledgments & Inspiration

Códice's architecture draws inspiration from these excellent open-source projects:

- **[Calibre-Web](https://github.com/janeczku/calibre-web)** (GPL-3.0) —
  Metadata extraction patterns, provider plugin architecture, and multi-format
  reader integration approach.

- **[Komga](https://github.com/gotson/komga)** (MIT) —
  Server-side page streaming architecture, metadata lock system, media analysis
  pipeline, comic/manga reader modes (RTL, webtoon, double-page spread), and
  OPDS feed implementation.

No code was directly copied from either project. Códice reimplements these
concepts in its own Go + Python + React stack.
```

---

## 8. Stack de Dependências

### Python Worker — Novas

| Pacote | Propósito | Substitui |
|:-------|:---------|:----------|
| `lxml` | XML/OPF parsing (EPUB) | PyMuPDF para metadados |
| `pypdf` | PDF metadata | PyMuPDF para metadados |
| `rarfile` | CBR/RAR reading | — |
| `mutagen` | Audio ID3/MP4 tags | — |
| `Pillow` | Thumbnail generation | — |
| `python-magic` | MIME detection | — |
| `aiohttp` | Async HTTP providers | `requests` |

### Go Backend — Novas

| Pacote | Propósito |
|:-------|:---------|
| `archive/zip` (stdlib) | CBZ page streaming |
| `go-chi/httprate` | Rate limiting |
| Restante é stdlib | `http.ServeContent`, etc. |

### Frontend

| Pacote | Ação |
|:-------|:-----|
| `jszip` | **Remover** (server-side streaming) |
| `react-pdf` / `pdfjs-dist` | Manter |
| `react-reader` / `epubjs` | Manter |
| `react-markdown` | Adicionar (Sprint 5) |
