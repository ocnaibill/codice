# Códice 📚

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

## 🧪 Running Tests

```bash
# Run all tests (backend Go + worker Python + frontend JS)
make test-all

# Run tests for a specific stack only
make test-backend    # go test ./... — discovers all *_test.go
make test-worker     # pytest tests/ — discovers all test_*.py
make test-frontend   # npx vitest run — discovers all *.test.js / *.test.jsx
```

**Current coverage:**
- Backend: 22 Go tests (config, auth middleware, auth handler, page handler)
- Frontend: 5 JS tests (api.js helpers — authenticatedUrl, wsUrl)
- Worker: pytest setup ready for extractors and providers

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