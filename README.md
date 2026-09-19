# Códice 📚

> Estado atual, decisões e próximos passos: [documentação do projeto](docs/README.md). O histórico abaixo descreve os sprints iniciais; consulte também a [validação da base de 19/09/2026](docs/Codice_Validacao_Base_2026-09-19.md).

A complete, modern, and open-source system for managing and consuming digital libraries (Homelab). Códice supports fiction books (EPUB, PDF), manga/comic collections (CBZ, CBR), plain text (TXT, MD), and audiobooks — with a specialized metadata extraction pipeline and OPDS 1.2 catalog for mobile app compatibility.

## 🏗️ Architecture

Códice is built with a focus on performance, resilience, and low resource consumption, ideal for running on local servers (Homelabs).

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Backend (API & OPDS)** | Go (Chi router, PostgreSQL, Redis) | REST API, auth, page streaming, file serving, OPDS 1.2 |
| **Worker** | Python (PyMuPDF, lxml, Redis Streams) | Metadata extraction + enrichment via external providers |
| **Database** | PostgreSQL (with GIN indexes) | Works, tags, authors, user progress, media status |
| **Queue** | Redis Streams + Redis PubSub | Async ingestion tasks + real-time WebSocket events |
| **Frontend** | React (Vite) + TanStack Query + Zustand + Tailwind CSS | UI: library, readers (PDF/EPUB/CBZ/TXT/MD/audio), metadata editor |

## ✨ Features

- **Multi-format readers**: PDF, EPUB, CBZ, CBR, TXT, MD, MP3, M4A, OGG, FLAC
- **Server-side page streaming**: CBZ/CBR pages served individually via `archive/zip`, no full download to browser
- **4 reading modes**: LTR, RTL (manga), Webtoon (scroll), Double-page spread
- **Metadata extraction**: Format-specific extractors (EPUB OPF, PDF Document Info, ComicInfo.xml, filename-based)
- **Metadata enrichment**: Google Books API, OpenLibrary, ComicVine (pluggable provider registry)
- **Media status lifecycle**: UNKNOWN → ANALYZING → READY / ERROR (inspired by Komga)
- **OPDS 1.2 catalog**: Compatible with Panels, Chunky, Mihon, KOReader, Moon+ Reader
- **Security**: JWT auth, rate limiting, controlled registration, environment-based middleware
- **Server-side search & pagination**: SQL LIKE queries, paginated responses

## 🔐 Login com Authentik (LDAP)

O Códice pode autenticar quem já tem conta no seu [Authentik](https://goauthentik.io) pelo protocolo LDAP. É opcional e vem desligado. O dono da instância **sempre** entra por senha local, e quem entra pelo diretório vira **leitor**: só o dono promove alguém a administrador.

O Authentik não fala LDAP sozinho. Quem fala é o **outpost LDAP**, um contêiner à parte que lê os usuários do Authentik e os apresenta como um diretório:

```
Códice (API) ──LDAPS──▶ outpost LDAP ──HTTPS (saída)──▶ seu Authentik
```

O outpost pode rodar em qualquer máquina que alcance o Authentik por HTTPS. Num homelab, o mais simples é rodá-lo na mesma máquina do Códice, publicando as portas só no `127.0.0.1`. **Não exponha 389/636 na rede nem na internet:** é a porta de login das suas contas.

Os exemplos usam `authentik.example.com` e `dc=ldap,dc=example,dc=com`; troque pelos seus. Feito e testado com o Authentik 2024.12; em outras versões os nomes dos campos podem mudar um pouco.

### 1. No Authentik

1. **Provider** (*Applications → Providers → Create → LDAP Provider*):
   - **Bind mode** e **Search mode**: *Direct*. No modo em cache, quem foi desativado pode continuar entrando até o cache renovar.
   - **Code-based MFA support**: desligado (o Códice não envia código TOTP junto da senha).
   - **Bind flow**: `default-authentication-flow` para começar; se o bind falhar por causa de captcha ou MFA no fluxo, crie um fluxo próprio só com identificação, senha e login. **Unbind flow**: `default-invalidation-flow`.
   - **Base DN**: um nome interno qualquer, por exemplo `dc=ldap,dc=example,dc=com`. Não precisa existir no DNS.
2. **Application** (*Applications → Applications → Create*): crie uma aplicação (por exemplo `Códice`) e ligue a ela o provider. Sem isso os binds são negados. Se quiser limitar quem entra, use os *Policy bindings* da aplicação.
3. **Conta de serviço** (*Directory → Users → Create Service Account*), por exemplo `codice-svc`. O Authentik mostra uma senha (token de app) **uma única vez**: guarde-a. No provider, em permissões, dê a essa conta a permissão **Search full LDAP directory**; sem ela a conta só enxerga a si mesma e ninguém consegue entrar.
4. **Certificado** (*System → Certificates → Generate*): gere um com o nome que o Códice usará para chegar ao outpost (`localhost`, se ele roda na mesma máquina), marcando o mesmo nome como nome alternativo (SAN). No provider, escolha esse certificado em **Certificate** e ponha o mesmo nome em **TLS Server name**. Baixe o certificado público (**não** a chave): será o `LDAP_CA_FILE` do Códice. Se for testar com `openssl`, use `-partial_chain`: o emissor dos certificados gerados pelo Authentik não vem no arquivo. O Códice exige LDAPS ou StartTLS e confere o certificado e o nome.
5. **Outpost** (*Applications → Outposts → Create*): tipo **LDAP**, selecionando o provider. Copie o token do outpost. Ele só serve ao contêiner do outpost; não vai para o Códice.

### 2. No servidor do Códice

Hoje a API do Códice roda direto na máquina (veja "Como rodar" em [`docs/README.md`](docs/README.md)): o `docker-compose.full.yml` referencia Dockerfiles que ainda não existem no repositório, então a pilha completa em contêineres ainda não sobe. O caminho abaixo é o que foi testado: **API na máquina, outpost em contêiner**, num terminal só do dono.

**a) O outpost**, numa pasta fora do repositório (o token nunca deve entrar num repositório):

```bash
mkdir -p ~/authentik-outpost && cd ~/authentik-outpost
touch outpost.env && chmod 600 outpost.env
```

Escreva você mesmo, com `nano outpost.env`, o endereço e o token do outpost (passo 5 acima):

```
AUTHENTIK_HOST=https://authentik.example.com
AUTHENTIK_TOKEN=<token do outpost>
```

Crie o `docker-compose.yml` ao lado. O outpost publica as portas **só no localhost**:

```yaml
services:
  ldap-outpost:
    image: ghcr.io/goauthentik/ldap:${AUTHENTIK_TAG:?defina AUTHENTIK_TAG no arquivo .env desta pasta}
    restart: unless-stopped
    env_file: ./outpost.env
    ports:
      - "127.0.0.1:3389:3389"   # LDAP (com StartTLS)
      - "127.0.0.1:6636:6636"   # LDAPS
```

`AUTHENTIK_TAG` deve ser **exatamente a versão do seu Authentik** (aparece no menu da administração; o outpost precisa ser da mesma versão). Ela vai num `.env` desta pasta, sem segredo:

```bash
echo "AUTHENTIK_TAG=2024.12.1" > .env
docker compose up -d
```

Confira o log (`docker compose logs ldap-outpost`): devem aparecer `Starting LDAP server` e `Starting LDAP SSL server`. Se aparecerem avisos `websocket: bad handshake`, veja a tabela no fim.

**b) O certificado.** No passo 4, dê ao certificado o nome `localhost` (é o nome pelo qual a API vai chegar ao outpost, que roda na mesma máquina). Guarde o certificado público baixado, por exemplo em `~/authentik-outpost/localhost.pem`.

**c) O `.env` da raiz do Códice** (escreva você mesmo; o arquivo não vai para o git). O caminho do certificado é absoluto:

```
LDAP_URL=ldaps://localhost:6636
LDAP_BIND_DN=cn=codice-svc,ou=users,dc=ldap,dc=example,dc=com
LDAP_BIND_PASSWORD=<senha da conta de serviço>
LDAP_BASE_DN=ou=users,dc=ldap,dc=example,dc=com
LDAP_USER_FILTER=(cn={username})
LDAP_USERNAME_ATTR=cn
LDAP_ID_ATTR=uid
LDAP_EMAIL_ATTR=mail
LDAP_CA_FILE=/caminho/absoluto/para/localhost.pem
```

(`LDAP_BIND_PASSWORD_FILE=/caminho/arquivo` lê a senha de um arquivo, em vez de pô-la no `.env`.) Reinicie a API.

Três detalhes que importam (foram confirmados num Authentik real):
- **`LDAP_BASE_DN` termina em `ou=users`.** O Authentik cria para cada usuário um *grupo virtual* com o mesmo nome e o mesmo `uid`; buscar na árvore inteira acha duas entradas, e o Códice recusa a ambiguidade em vez de adivinhar.
- **`LDAP_ID_ATTR=uid`.** É o identificador estável a que a conta é ligada. O padrão do Códice (`entryUUID`) não existe no Authentik. Como a ligação é por esse valor, renomear alguém no Authentik não cria conta nova nem perde a antiga.
- **`LDAP_BIND_DN` mora sob `ou=users`** e usa o `cn` da conta de serviço.

**Quando o Códice tiver imagens de contêiner.** O `docker-compose.full.yml` já repassa as variáveis `LDAP_*` ao backend, monta a pasta `./ldap` (no `.gitignore`) para ler a senha e o certificado, e tem o outpost como serviço opcional do perfil `ldap` (`docker compose -f docker-compose.full.yml --profile ldap up -d`), sem publicar porta e acessível ao backend como `ldap-outpost:6636`. Nesse caso, o certificado do passo 4 deve ter o nome `ldap-outpost`, o token do outpost vai em `ldap/outpost.env`, e `LDAP_BIND_PASSWORD_FILE` e `LDAP_CA_FILE` apontam para `/app/ldap/...`. **Essa parte ainda não foi testada**, porque depende dos Dockerfiles.

### 3. Conferir

1. Nos logs da API deve aparecer `LDAP sign-in enabled: ldaps://localhost:6636`. Se aparecer `LDAP configuration: …`, a API parou por uma configuração incompleta ou insegura, e a mensagem diz o quê.
2. Entre como **dono**, abra *Administração → Login externo* e clique em **Testar conexão**. Se falhar, o motivo real aparece no log da API (`LDAP connection test failed: …`).
3. Na mesma aba, decida a política: **"Permitir criar conta no primeiro login"** (desligada por padrão: sem ela só entra quem já tem conta ou convite) e por quanto tempo uma conta do diretório pode ficar entrando sem o Authentik reconfirmá-la (24 h por padrão; quem for desativado no Authentik perde o acesso no máximo depois desse prazo).
4. Entre com um usuário do Authentik. Em *Contas* ele aparece como leitor, com o selo "Diretório".

Bloquear uma conta no Códice vale na hora, sem esperar o Authentik. Se já existir uma conta local com o mesmo nome (ou o mesmo e-mail) de alguém do Authentik, o Códice não junta as duas sozinho: pede também a senha local, e só então liga a conta ao diretório. Depois disso a conta entra só pelo diretório.

### Se algo der errado

| Sintoma | Causa provável |
|---|---|
| A API não sobe: `LDAP configuration: …` | `ldap://` sem StartTLS, falta a conta de serviço ou a base, ou o filtro não tem `{username}`. |
| "Testar conexão" falha | Veja `LDAP connection test failed: …` no log. Costuma ser certificado que não cobre o nome usado em `LDAP_URL`, `LDAP_CA_FILE` errado ou senha da conta de serviço recusada. |
| Ninguém consegue entrar, ou `more than one entry matches` no log | `LDAP_BASE_DN` sem `ou=users`, ou a conta de serviço sem a permissão *Search full LDAP directory*. |
| `Invalid credentials (49)` numa busca de teste | Senha ou DN da conta de serviço errados. Um `502` no log do outpost no mesmo instante é o proxy do Authentik falhando, e passa numa nova tentativa. |
| O outpost mostra `websocket: bad handshake` | O proxy na frente do Authentik não repassa WebSocket (no nginx: `Upgrade` e `Connection`; no Cloudflare: WebSockets ligados e nenhuma regra bloqueando `/ws/`). O LDAP funciona assim mesmo, mas o Authentik mostra o outpost como desconectado e mudanças de configuração exigem reiniciá-lo. |
| Diretório fora do ar | Quem entra pelo diretório recebe um aviso próprio (não um "senha errada"). Contas locais, inclusive a do dono, seguem entrando. |

Mais detalhes, decisões e o que foi testado: [`docs/README.md`](docs/README.md).

## 🧪 Running Tests

```bash
# Run all tests (backend Go + worker Python + frontend JS)
make test-all

# Run tests for a specific stack only
make test-backend    # go test ./... — discovers all *_test.go
make test-worker     # pytest tests/ — discovers all test_*.py
make test-frontend   # npx vitest run — discovers all *.test.js / *.test.jsx
```

**Latest validation (2026-09-19):**
- Backend: 232 passing cases/subcases, including PostgreSQL integration tests
- Frontend: 54 passing tests
- Worker: 82 passing tests
- These are passing test counts, not coverage percentages. Integration tests require an isolated `TEST_DATABASE_URL`; see `docs/README.md`.

## 🙏 Inspirations & References

This project studies and adapts architectural patterns from two excellent open-source projects. **No code was copied** — only design approaches are used as reference.

### 📚 Calibre-Web ([GPL-3.0](https://github.com/janeczku/calibre-web))
- **Metadata provider pattern**: Pluggable providers (Google Books, OpenLibrary, ComicVine) with structured dataclass responses
- **Format-specific extractors**: Dedicated extractors per format (EPUB OPF via lxml, PDF via PyMuPDF, ComicInfo.xml, etc.)
- **Cover handling**: Local caching, download from providers, fallback to first page

### 📖 Komga ([MIT](https://github.com/gotson/komga))
- **Server-side page streaming**: CBZ/CBR pages served individually from ZIP archives (never sends full archive to browser)
- **Media status lifecycle**: `UNKNOWN → QUEUED → ANALYZING → READY | ERROR | OUTDATED`
- **Metadata lock columns**: `title_lock`, `author_lock`, `cover_lock` — manually edited fields aren't overwritten on rescan
- **ComicInfo.xml parsing**: Series, Writer, Penciller, genre tags extraction

> [!NOTE]
> Códice is licensed under **AGPL-3.0**, which is compatible with GPL-3.0 (Calibre-Web) and MIT (Komga).

## 🚀 Project Status

Currently in active development. All 6 planned sprints have been completed (Sprint 0 through 6). See the [Issues](https://github.com/ocnaibill/codice/issues) for upcoming tasks.

## 📄 License

This project is licensed under **GNU AGPLv3** - see the [LICENSE](LICENSE) file for details. Códice is copyleft: feel free to host, use, and modify, as long as any modified source code remains public.
