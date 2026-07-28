x# Códice — Tarefas Pendentes (para agente executor)

> **Documento auto-contido**. Este documento contém todas as informações necessárias para implementar cada tarefa sem precisar de contexto adicional. Leia completamente antes de começar qualquer implementação.

---

## Contexto do Projeto

**Códice** é uma plataforma self-hosted de gerenciamento de livros/quadrinhos/mangás. Stack: Go (backend), Python (workers assíncronos), React/Vite (frontend).

- **Repo**: `/Users/bianco/repos/codice`
- **Backend Go**: `/Users/bianco/repos/codice/backend`
- **Worker Python**: `/Users/bianco/repos/codice/worker`
- **Frontend React**: `/Users/bianco/repos/codice/frontend`

### Arquitetura de Extratores

O worker Python tem um sistema de extratores por formato de arquivo. Cada formato tem um extrator dedicado em `worker/extractors/`:

```
worker/extractors/
├── __init__.py              # Exporta todos os extratores
├── base.py                  # ExtractedMetadata (dataclass) + BaseExtractor (ABC)
├── epub_extractor.py        # EPUB → zipfile + XML OPF
├── pdf_extractor.py         # PDF → pypdf
├── cbz_extractor.py         # CBZ → zipfile + ComicInfo.xml
├── cbr_extractor.py         # CBR → rarfile + ComicInfo.xml
├── txt_extractor.py         # TXT/MD → filename
└── audio_extractor.py       # MP3/M4A/FLAC → mutagen
```

Cada extrator:
1. Implementa `can_extract(file_path) → bool`
2. Implementa `extract(file_path, covers_dir) → ExtractedMetadata`
3. É registrado em `worker/main.py` na função `register_extractors()`
4. É exportado em `worker/extractors/__init__.py`

### Dataclass de Metadados

```python
# worker/extractors/base.py (NÃO MODIFICAR)
@dataclass
class ExtractedMetadata:
    title: str = ""
    author: str = ""
    series: str = ""
    series_index: float = 0.0
    isbn: str = ""
    language: str = ""
    publisher: str = ""
    publication_date: str = ""
    description: str = ""
    tags: List[str] = field(default_factory=list)
    page_count: int = 0
    cover_path: str = ""     # Relative path: "/covers/cover_xxx.jpg"
    format: str = ""         # "pdf", "epub", "cbz", "mobi", etc.
    raw: dict = field(default_factory=dict)
```

### Projetos de Inspiração

- **Calibre-Web** (GPL-3.0): https://github.com/janeczku/calibre-web — referência para extração multi-formato
- **Komga** (MIT): https://github.com/gotson/komga — referência para page streaming e metadata locks

---

## Tarefa 1: Suporte a MOBI/AZW (EXTH parser)

### Motivação

MOBI e AZW3 (KF8) são formatos Amazon Kindle. Embora a Amazon esteja migrando para EPUB, ainda existem milhões de arquivos .mobi/.azw/.azw3 em circulação. O Calibre-Web suporta estes formatos e o Códice deveria também.

### O que precisa ser feito

Criar um `MobiExtractor` que leia metadados de arquivos MOBI/AZW3 e extraia a imagem de capa.

### Referência: como o Calibre-Web faz

No Calibre-Web, o parsing de MOBI é feito via um módulo customizado que lê o header binário EXTH. O fluxo é:

1. Abrir o arquivo binário
2. Ler o PalmDB header (primeiros 78 bytes) para achar o offset dos records
3. Ler o MOBI header (começa no primeiro record) para pegar `encoding`, `title_offset`, `title_length`
4. Ler o bloco EXTH (embedded extended header) que contém os metadados como key-value pairs
5. Os EXTH record types relevantes:
   - `100` → author
   - `101` → publisher
   - `103` → description
   - `104` → ISBN
   - `105` → subject/tags (pode repetir)
   - `106` → publication date
   - `109` → rights
   - `503` → title atualizado (sobrescreve o do header)
   - `524` → language
6. A capa é a imagem no index `first_image_index` dos records do PalmDB

### Arquivos a criar/modificar

| Arquivo | Ação |
|:--------|:-----|
| `worker/extractors/mobi_extractor.py` | **CRIAR** — novo extrator |
| `worker/extractors/__init__.py` | **MODIFICAR** — adicionar `from .mobi_extractor import MobiExtractor` |
| `worker/main.py` | **MODIFICAR** — adicionar `MobiExtractor()` em `register_extractors()` (L62-69) |
| `backend/cmd/api/main.go` | **VERIFICAR** — se `.mobi`/`.azw`/`.azw3` são aceitos no upload (verificar o handler de upload em `backend/internal/handlers/upload.go` L43) |
| `backend/internal/handlers/upload.go` | **MODIFICAR** — adicionar `.mobi`, `.azw`, `.azw3` à lista de extensões permitidas (L43) |
| `frontend/src/features/reader/components/Reader.jsx` | **VERIFICAR** — MOBI não precisa de viewer dedicado pois idealmente seria convertido para EPUB pelo worker; mas como fallback, redirecionar para download |

### Implementação passo a passo

**Passo 1: Criar `worker/extractors/mobi_extractor.py`**

```python
"""MOBI/AZW metadata extractor via EXTH binary header parsing.

Inspired by Calibre-Web's MOBI extraction approach.
Parses the PalmDB + MOBI + EXTH headers directly from the binary file.
No external dependencies required beyond stdlib.
"""
import os
import struct
from typing import Optional
from .base import BaseExtractor, ExtractedMetadata


class MobiExtractor(BaseExtractor):
    """Extracts metadata from MOBI, AZW, and AZW3 (KF8) files."""

    MOBI_EXTS = {'.mobi', '.azw', '.azw3', '.prc'}

    # EXTH record type IDs → metadata field names
    EXTH_FIELDS = {
        100: 'author',
        101: 'publisher',
        103: 'description',
        104: 'isbn',
        105: 'tag',        # Can repeat (subject/genre)
        106: 'publication_date',
        503: 'title',      # Updated title (overrides header title)
        524: 'language',
    }

    def can_extract(self, file_path: str) -> bool:
        ext = os.path.splitext(file_path)[1].lower()
        return ext in self.MOBI_EXTS

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(
            format=os.path.splitext(file_path)[1].lstrip('.').lower()
        )
        tags = []

        try:
            with open(file_path, 'rb') as f:
                # 1. Parse PalmDB header
                content = f.read()
                palm_header = self._parse_palm_header(content)
                if not palm_header:
                    raise ValueError("Invalid PalmDB header")

                records = palm_header['records']
                if not records:
                    raise ValueError("No records found in PalmDB")

                # 2. Parse MOBI header (starts at first record)
                first_record_offset = records[0]
                mobi = self._parse_mobi_header(content, first_record_offset)

                # 3. Get title from MOBI header
                if mobi.get('title'):
                    meta.title = mobi['title']

                # 4. Parse EXTH header if present
                if mobi.get('has_exth'):
                    exth_offset = first_record_offset + 16 + mobi['header_length']
                    exth_data = self._parse_exth(content, exth_offset)

                    if 'title' in exth_data:
                        meta.title = exth_data['title']  # EXTH 503 overrides
                    if 'author' in exth_data:
                        meta.author = exth_data['author']
                    if 'publisher' in exth_data:
                        meta.publisher = exth_data['publisher']
                    if 'description' in exth_data:
                        meta.description = exth_data['description']
                    if 'isbn' in exth_data:
                        meta.isbn = exth_data['isbn']
                    if 'publication_date' in exth_data:
                        meta.publication_date = exth_data['publication_date']
                    if 'language' in exth_data:
                        meta.language = exth_data['language']
                    if 'tags' in exth_data:
                        tags = exth_data['tags']

                # 5. Extract cover image
                first_image = mobi.get('first_image_index')
                if first_image and first_image < len(records):
                    img_start = records[first_image]
                    img_end = records[first_image + 1] if first_image + 1 < len(records) else len(content)
                    image_data = content[img_start:img_end]

                    # Verify it's actually an image (JPEG or PNG magic bytes)
                    if image_data[:2] == b'\xff\xd8' or image_data[:4] == b'\x89PNG':
                        meta.cover_path = self._save_cover(image_data, file_path, covers_dir)

        except Exception as e:
            print(f"   ⚠️ MOBI extraction error: {e}")

        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"
        if tags:
            meta.tags = tags

        return meta

    def _parse_palm_header(self, content: bytes) -> Optional[dict]:
        """Parse PalmDB header to get record offsets."""
        if len(content) < 78:
            return None

        # Number of records: bytes 76-77 (big-endian unsigned short)
        num_records = struct.unpack_from('>H', content, 76)[0]
        records = []

        for i in range(num_records):
            offset_pos = 78 + (i * 8)
            if offset_pos + 4 > len(content):
                break
            record_offset = struct.unpack_from('>I', content, offset_pos)[0]
            records.append(record_offset)

        return {'num_records': num_records, 'records': records}

    def _parse_mobi_header(self, content: bytes, offset: int) -> dict:
        """Parse MOBI header for title and image index."""
        result = {}

        # PalmDOC header: compression(2) + unused(2) + text_length(4) + record_count(2) + ...
        # MOBI header starts at offset + 16
        mobi_offset = offset + 16

        if mobi_offset + 132 > len(content):
            return result

        # Check 'MOBI' magic at mobi_offset
        magic = content[mobi_offset:mobi_offset + 4]
        if magic != b'MOBI':
            return result

        # Header length: offset+4, 4 bytes
        header_length = struct.unpack_from('>I', content, mobi_offset + 4)[0]
        result['header_length'] = header_length

        # Encoding: offset+8, 4 bytes (1252=cp1252, 65001=utf-8)
        encoding_raw = struct.unpack_from('>I', content, mobi_offset + 8)[0]
        result['encoding'] = 'utf-8' if encoding_raw == 65001 else 'cp1252'

        # Full title offset: mobi_offset+84, 4 bytes (relative to first record)
        title_offset = struct.unpack_from('>I', content, mobi_offset + 84)[0]
        title_length = struct.unpack_from('>I', content, mobi_offset + 88)[0]

        if title_offset and title_length:
            abs_title_offset = offset + title_offset
            if abs_title_offset + title_length <= len(content):
                raw_title = content[abs_title_offset:abs_title_offset + title_length]
                try:
                    result['title'] = raw_title.decode(result['encoding']).strip()
                except (UnicodeDecodeError, LookupError):
                    result['title'] = raw_title.decode('utf-8', errors='replace').strip()

        # EXTH flags: mobi_offset+128, 4 bytes
        exth_flags = struct.unpack_from('>I', content, mobi_offset + 128)[0]
        result['has_exth'] = bool(exth_flags & 0x40)

        # First image index: mobi_offset+108, 4 bytes
        if mobi_offset + 112 <= len(content):
            first_image = struct.unpack_from('>I', content, mobi_offset + 108)[0]
            if first_image != 0xFFFFFFFF:
                result['first_image_index'] = first_image

        return result

    def _parse_exth(self, content: bytes, offset: int) -> dict:
        """Parse EXTH header for rich metadata fields."""
        result = {}
        tags = []

        if offset + 12 > len(content):
            return result

        magic = content[offset:offset + 4]
        if magic != b'EXTH':
            return result

        # header_length = struct.unpack_from('>I', content, offset + 4)[0]
        record_count = struct.unpack_from('>I', content, offset + 8)[0]

        pos = offset + 12
        for _ in range(min(record_count, 200)):  # Safety cap
            if pos + 8 > len(content):
                break

            record_type = struct.unpack_from('>I', content, pos)[0]
            record_length = struct.unpack_from('>I', content, pos + 4)[0]

            if record_length < 8:
                break

            data_length = record_length - 8
            if pos + 8 + data_length > len(content):
                break

            raw_data = content[pos + 8:pos + 8 + data_length]

            if record_type in self.EXTH_FIELDS:
                field_name = self.EXTH_FIELDS[record_type]
                try:
                    value = raw_data.decode('utf-8').strip()
                except UnicodeDecodeError:
                    value = raw_data.decode('cp1252', errors='replace').strip()

                if field_name == 'tag':
                    tags.append(value)
                else:
                    result[field_name] = value

            pos += record_length

        if tags:
            result['tags'] = tags

        return result
```

**Passo 2: Registrar o extrator**

Em `worker/extractors/__init__.py`, adicionar:
```python
from .mobi_extractor import MobiExtractor
```

Em `worker/main.py`, na função `register_extractors()` (~L62), adicionar `MobiExtractor()` na lista.

**Passo 3: Aceitar MOBI no upload**

Em `backend/internal/handlers/upload.go` L43, a validação de extensão é:
```go
if ext != ".pdf" && ext != ".epub" && ext != ".cbz" {
```
Precisa virar:
```go
if ext != ".pdf" && ext != ".epub" && ext != ".cbz" && ext != ".cbr" && ext != ".mobi" && ext != ".azw" && ext != ".azw3" {
```

> [!IMPORTANT]
> Verificar se `.cbr` já está nessa lista — pode ser que já tenha sido adicionado na Sprint 5. Se sim, apenas adicionar `.mobi`, `.azw`, `.azw3`.

Aplicar a mesma alteração no `HandleBulkImport` (~L167) que tem validação semelhante.

**Passo 4: Frontend — Fallback para download**

Em `frontend/src/features/reader/components/Reader.jsx`, no switch/if que seleciona o viewer baseado em `format`, adicionar:
```jsx
case 'mobi':
case 'azw':
case 'azw3':
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4 text-zinc-400">
      <p>MOBI/AZW files cannot be viewed in the browser.</p>
      <a href={authenticatedUrl(book.fileUrl)}
         download
         className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-500">
        Download File
      </a>
    </div>
  );
```

> [!NOTE]
> Futuramente podemos converter MOBI → EPUB no worker (usando Calibre CLI `ebook-convert`) e oferecer o EPUB para visualização. Mas isso é uma tarefa separada e mais complexa.

**Passo 5: Testes**

Criar `worker/tests/test_mobi_extractor.py` seguindo o padrão dos testes existentes em `worker/tests/test_extractors.py`. Testar:
- `can_extract` retorna True para `.mobi`, `.azw`, `.azw3`, `.prc`
- `can_extract` retorna False para `.pdf`, `.epub`
- `extract` com arquivo MOBI inválido retorna fallback (título do filename, "Unknown Author")

### Critérios de Aceite

- [ ] `MobiExtractor` parseia EXTH headers corretamente para título, autor, isbn, publisher, descrição, tags, language
- [ ] Capa é extraída se presente no record index
- [ ] Upload aceita `.mobi`, `.azw`, `.azw3`
- [ ] Frontend mostra fallback de download para formatos MOBI
- [ ] Testes unitários passam
- [ ] Não quebra nenhum extrator existente

### O que NÃO fazer

- ❌ NÃO instalar Calibre como dependência
- ❌ NÃO converter MOBI para EPUB nesta tarefa (é escopo futuro)
- ❌ NÃO modificar `base.py` — a dataclass `ExtractedMetadata` não precisa de campos novos
- ❌ NÃO adicionar dependências externas — o parser EXTH usa apenas `struct` (stdlib)

---

## Tarefa 2: epub_extractor — trocar `xml.etree` por `lxml`

### Motivação

O `epub_extractor.py` atualmente usa `xml.etree.ElementTree` (stdlib) para parsear o OPF/Dublin Core XML dos EPUBs. O `lxml` (que **já está no requirements.txt** como `lxml>=5.0`) é muito mais robusto para lidar com:
- XMLs malformados (common em EPUBs de fontes duvidosas)
- Entidades HTML não escapadas
- Namespaces inconsistentes
- Encoding declarado incorretamente

### Arquivo a modificar

- `worker/extractors/epub_extractor.py` — mudar as linhas onde `xml.etree.ElementTree` é importado e usado

### O que mudar

A mudança é cirúrgica — apenas substituir o parser, não a lógica. O `lxml.etree` tem API quase idêntica ao `xml.etree.ElementTree`.

**Localização**: [epub_extractor.py L63](file:///Users/bianco/repos/codice/worker/extractors/epub_extractor.py#L63)

**Antes (xml.etree)**:
```python
import xml.etree.ElementTree as ET
# ...
opf_data = zf.read(opf_path)
root = ET.fromstring(opf_data)
```

**Depois (lxml.etree)**:
```python
from lxml import etree
# ...
opf_data = zf.read(opf_path)
root = etree.fromstring(opf_data, parser=etree.XMLParser(recover=True, encoding='utf-8'))
```

A flag `recover=True` é o diferencial — faz o `lxml` tentar recuperar XMLs malformados em vez de crashar.

### Passo a passo completo

1. Na função `_parse_opf` (~L62), trocar:
   ```python
   import xml.etree.ElementTree as ET
   ```
   por:
   ```python
   from lxml import etree
   ```

2. Trocar `ET.fromstring(opf_data)` por:
   ```python
   root = etree.fromstring(opf_data, parser=etree.XMLParser(recover=True, encoding='utf-8'))
   ```

3. No método `_extract_cover` (~L106), há um `import re` que está OK. Mas se houver algum `ET.` referenciado, trocar por `etree.`.

4. O resto do código (iteração, `.tag`, `.get()`, `.text`) funciona **identicamente** com `lxml.etree`. Nenhuma outra mudança é necessária na lógica.

5. No fallback de `container.xml` (~L29), há um uso de regex para parsear `container.xml`. Pode opcionalmente trocar por `etree.fromstring` também:
   ```python
   # Antes:
   container_xml = zf.read('META-INF/container.xml').decode('utf-8')
   import re
   match = re.search(r'full-path="([^"]+)"', container_xml)

   # Depois (opcional, mais robusto):
   container_data = zf.read('META-INF/container.xml')
   container_root = etree.fromstring(container_data, parser=etree.XMLParser(recover=True))
   ns = {'c': 'urn:oasis:names:tc:opendocument:xmlns:container'}
   rootfile = container_root.find('.//c:rootfile', ns)
   if rootfile is not None:
       opf_path = rootfile.get('full-path')
   ```

### Critérios de Aceite

- [ ] `lxml.etree` usado em vez de `xml.etree.ElementTree`
- [ ] `recover=True` no XMLParser
- [ ] Testes existentes em `worker/tests/test_extractors.py` continuam passando
- [ ] EPUBs válidos continuam extraindo metadados corretamente

### O que NÃO fazer

- ❌ NÃO mudar a lógica de extração — apenas o parser
- ❌ NÃO adicionar `lxml` ao requirements.txt — **já está lá** (`lxml>=5.0`)
- ❌ NÃO remover os fallbacks (filename title, "Unknown Author") que existem no final do `extract()`

---

## Tarefa 3: Migration 009 — tabelas `work_identifiers` e `media_pages`

### Motivação

O schema atual coloca tudo na tabela `works` (via migration 008). Isso funciona, mas duas áreas se beneficiariam de tabelas relacionais separadas:

1. **`work_identifiers`** — Um livro pode ter múltiplos identificadores (ISBN-10, ISBN-13, Goodreads ID, ASIN, Google Books ID, ComicVine ID, OpenLibrary ID). Colocar tudo em colunas da `works` não escala. Uma tabela relacional 1:N é melhor.

2. **`media_pages`** — Para comics/mangás, queremos armazenar metadados por página (tipo: story/ad/filler, bookmark, reading direction). Komga usa isso para sua feature de "page metadata".

### Schema atual relevante (para referência)

```sql
-- Tabela works (migration 001 + 008)
CREATE TABLE works (
    id SERIAL PRIMARY KEY,
    original_title VARCHAR(255) NOT NULL,
    author_id INT REFERENCES person(id),
    original_release_year INT,
    series_id INT REFERENCES series(id),
    file_path TEXT,                              -- migration 004
    format VARCHAR(16),                          -- migration 008
    media_status VARCHAR(20) DEFAULT 'UNKNOWN',  -- migration 008
    isbn VARCHAR(32),                            -- migration 008 (vai ser deprecado em favor de work_identifiers)
    -- ... outros campos da 008
);
```

### Arquivo a criar

- `backend/migrations/009_create_identifier_and_page_tables.sql`

### Conteúdo da migration

```sql
-- Migration 009: Create work_identifiers and media_pages tables
-- work_identifiers: stores multiple identifiers per work (ISBN, Goodreads, ASIN, etc.)
-- media_pages: stores per-page metadata for comics/manga (type, bookmark, etc.)

-- 1. Work Identifiers (1:N relationship with works)
CREATE TABLE IF NOT EXISTS work_identifiers (
    id SERIAL PRIMARY KEY,
    work_id INT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    identifier_type VARCHAR(32) NOT NULL,  -- 'isbn10', 'isbn13', 'goodreads', 'asin', 'google_books', 'comicvine', 'openlibrary', 'anilist'
    identifier_value VARCHAR(128) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(work_id, identifier_type, identifier_value)
);

CREATE INDEX IF NOT EXISTS idx_work_identifiers_work_id ON work_identifiers(work_id);
CREATE INDEX IF NOT EXISTS idx_work_identifiers_type_value ON work_identifiers(identifier_type, identifier_value);

-- 2. Media Pages (1:N relationship with works, for comics/manga)
CREATE TABLE IF NOT EXISTS media_pages (
    id SERIAL PRIMARY KEY,
    work_id INT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    page_number INT NOT NULL,             -- 0-indexed page number
    file_name VARCHAR(512),               -- original filename inside the archive (e.g., "page_001.jpg")
    media_type VARCHAR(16) DEFAULT 'image', -- 'image', 'text', 'other'
    page_type VARCHAR(32) DEFAULT 'story',  -- 'story', 'filler', 'advertisement', 'spread_left', 'spread_right', 'cover'
    width INT,                            -- pixel width (if analyzed)
    height INT,                           -- pixel height (if analyzed)
    file_size INT,                        -- bytes
    file_hash VARCHAR(64),                -- MD5 or SHA256 for deduplication
    UNIQUE(work_id, page_number)
);

CREATE INDEX IF NOT EXISTS idx_media_pages_work_id ON media_pages(work_id);

-- 3. Add comment explaining the isbn column in works is kept for backward compatibility
-- The isbn column in works (from migration 008) is kept as a denormalized convenience field.
-- The canonical source of identifiers is now work_identifiers.
COMMENT ON COLUMN works.isbn IS 'Denormalized convenience field. Canonical identifiers are in work_identifiers table.';
```

### Backend: registrar a migration para auto-run

Verificar como as migrations são executadas. Abrir `backend/internal/database/` e procurar o mecanismo de auto-migration. As migrations SQL são executadas em ordem numérica pelo `RunAutoMigrations(db)` chamado em `main.go` L46. A nova migration `009_...` será automaticamente executada no próximo restart.

### Worker: popular `work_identifiers` no pipeline

Após criar a migration, modificar `worker/db.py` (ou `worker/analyzer.py`, dependendo de qual salva metadados) para também inserir identifiers:

```python
# Quando ISBN é encontrado pelo extrator ou provider:
if metadata.isbn:
    cur.execute("""
        INSERT INTO work_identifiers (work_id, identifier_type, identifier_value)
        VALUES (%s, 'isbn', %s)
        ON CONFLICT (work_id, identifier_type, identifier_value) DO NOTHING;
    """, (work_id, metadata.isbn))

# Quando um provider retorna seu ID (ex: google_books_id):
if enriched.source == 'google_books' and enriched.raw.get('google_id'):
    cur.execute("""
        INSERT INTO work_identifiers (work_id, identifier_type, identifier_value)
        VALUES (%s, 'google_books', %s)
        ON CONFLICT (work_id, identifier_type, identifier_value) DO NOTHING;
    """, (work_id, enriched.raw['google_id']))
```

### Worker: popular `media_pages` no extrator de comics

Modificar `worker/extractors/cbz_extractor.py` e `cbr_extractor.py` para retornar dados de páginas no campo `raw` do `ExtractedMetadata`:

```python
# No cbz_extractor, depois de listar as imagens:
meta.raw['pages'] = [
    {'page_number': i, 'file_name': name}
    for i, name in enumerate(images)
]
```

E no analyzer/db, inserir essas páginas:
```python
pages = metadata.raw.get('pages', [])
for page in pages:
    cur.execute("""
        INSERT INTO media_pages (work_id, page_number, file_name)
        VALUES (%s, %s, %s)
        ON CONFLICT (work_id, page_number) DO NOTHING;
    """, (work_id, page['page_number'], page['file_name']))
```

### Backend Go: endpoint para listar identificadores (opcional)

Se quiser expor os identificadores na API, adicionar ao `GetWorkByID` em `library.go`:

```go
// Fetch identifiers
idRows, _ := h.DB.Query("SELECT identifier_type, identifier_value FROM work_identifiers WHERE work_id = $1", id)
defer idRows.Close()
var identifiers []map[string]string
for idRows.Next() {
    var idType, idValue string
    idRows.Scan(&idType, &idValue)
    identifiers = append(identifiers, map[string]string{"type": idType, "value": idValue})
}
// Adicionar ao Work struct e ao JSON response
```

### Critérios de Aceite

- [ ] Migration `009` roda sem erros em PostgreSQL limpo e em PostgreSQL existente (idempotente com `IF NOT EXISTS`)
- [ ] `work_identifiers` aceita múltiplos identificadores por work
- [ ] `media_pages` aceita dados por página
- [ ] Constraint `UNIQUE` previne duplicatas
- [ ] `ON DELETE CASCADE` limpa identifiers e pages quando work é deletado
- [ ] Worker insere ISBNs e IDs de providers na tabela
- [ ] Coluna `isbn` da `works` continua funcionando (backward compat)

### O que NÃO fazer

- ❌ NÃO remover a coluna `isbn` da tabela `works` — manter como campo denormalizado para queries rápidas
- ❌ NÃO modificar as migrations anteriores (001-008) — migrations são imutáveis
- ❌ NÃO alterar `base.py` — usar o campo `raw: dict` para passar dados de páginas
- ❌ NÃO criar um endpoint separado para media_pages por enquanto — o page handler existente (`pages.go`) já lista páginas do ZIP em tempo real

---

## Tarefa 4 (Fix): `analyzer.save_metadata` não persiste metadados enriquecidos

### Motivação

Este é o bug mais grave da lista. O worker Python faz todo o trabalho de:
1. Extrair metadados locais (extratores)
2. Enriquecer via providers (Google Books, OpenLibrary, ComicVine)
3. Merge dos dados enriquecidos no objeto `metadata`

Mas na hora de salvar no banco, **só 5 campos são passados** ao `analyzer.save_metadata()`. Os campos `series`, `series_index`, `isbn`, `language`, `publisher`, `publication_date`, `description` e `tags` são **descartados silenciosamente**. Toda a Sprint 3 (providers) é efetivamente inútil.

### Arquivos a modificar

| Arquivo | Ação |
|:--------|:-----|
| `worker/main.py` (L157-164) | **MODIFICAR** — passar todos os campos ao `save_metadata` |
| `worker/analyzer.py` (L55-96) | **MODIFICAR** — gravar todos os campos no SQL UPDATE |

### Passo a passo

**Passo 1: Corrigir `worker/main.py` L157-164**

Atualmente:
```python
# 5. Save to database
analyzer.save_metadata(work_id, {
    'title': metadata.title,
    'author': metadata.author,
    'format': metadata.format,
    'page_count': metadata.page_count,
    'cover_path': metadata.cover_path,
})
```

Deve ser:
```python
# 5. Save to database (all extracted + enriched fields)
analyzer.save_metadata(work_id, {
    'title': metadata.title,
    'author': metadata.author,
    'format': metadata.format,
    'page_count': metadata.page_count,
    'cover_path': metadata.cover_path,
    'series': metadata.series,
    'series_index': metadata.series_index,
    'isbn': metadata.isbn,
    'language': metadata.language,
    'publisher': metadata.publisher,
    'publication_date': metadata.publication_date,
    'description': metadata.description,
    'tags': metadata.tags,
})
```

**Passo 2: Corrigir `worker/analyzer.py` método `save_metadata`**

O método atual só faz UPDATE de `original_title`, `format`, `page_count`. Precisa ser expandido para gravar todos os campos da migration 008.

Substituir o método `save_metadata` (L55-96) por:

```python
def save_metadata(self, work_id: int, metadata: dict):
    """Save extracted metadata to database, respecting lock columns."""

    # 1. Check which fields are locked (user manually edited → don't overwrite)
    locks = self.db.fetchone(
        "SELECT title_lock, author_lock, series_lock, cover_lock FROM works WHERE id = %s",
        (work_id,)
    )
    title_lock = locks[0] if locks else False
    author_lock = locks[1] if locks else False
    series_lock = locks[2] if locks else False
    cover_lock = locks[3] if locks else False

    # 2. Build dynamic UPDATE (skip locked fields)
    updates = []
    params = []

    if not title_lock and metadata.get('title'):
        updates.append("original_title = %s")
        params.append(metadata['title'])

    if metadata.get('format'):
        updates.append("format = %s")
        params.append(metadata['format'])

    if metadata.get('page_count'):
        updates.append("page_count = %s")
        params.append(metadata['page_count'])

    if not series_lock and metadata.get('series'):
        updates.append("series = %s")
        params.append(metadata['series'])

    if not series_lock and metadata.get('series_index'):
        updates.append("series_index = %s")
        params.append(metadata['series_index'])

    if metadata.get('isbn'):
        updates.append("isbn = %s")
        params.append(metadata['isbn'])

    if metadata.get('language'):
        updates.append("language = %s")
        params.append(metadata['language'])

    if metadata.get('publisher'):
        updates.append("publisher = %s")
        params.append(metadata['publisher'])

    if metadata.get('publication_date'):
        updates.append("publication_date = %s")
        params.append(metadata['publication_date'])

    if metadata.get('description'):
        updates.append("description = %s")
        params.append(metadata['description'])

    if updates:
        updates.append("updated_at = CURRENT_TIMESTAMP")
        query = f"UPDATE works SET {', '.join(updates)} WHERE id = %s"
        params.append(work_id)
        self.db.execute(query, tuple(params))

    # 3. Update author (with lock check)
    author = metadata.get('author')
    if author and author != 'Unknown Author' and not author_lock:
        author_query = """
            INSERT INTO person (name) VALUES (%s)
            ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
            RETURNING id
        """
        author_id = self.db.fetchone(author_query, (author,))
        if author_id:
            self.db.execute(
                "UPDATE works SET author_id = %s WHERE id = %s",
                (author_id[0], work_id)
            )

    # 4. Update cover (with lock check)
    cover_path = metadata.get('cover_path')
    if cover_path and not cover_lock:
        self.db.execute(
            """INSERT INTO editions (work_id, title, cover_url)
               VALUES (%s, %s, %s)
               ON CONFLICT (work_id)
               DO UPDATE SET cover_url = EXCLUDED.cover_url""",
            (work_id, metadata.get('title', ''), cover_path)
        )

    # 5. Save tags (Many-to-Many)
    tags = metadata.get('tags', [])
    if tags:
        for tag_name in tags:
            if not tag_name:
                continue
            self.db.execute(
                "INSERT INTO tags (name) VALUES (%s) ON CONFLICT (name) DO NOTHING",
                (tag_name,)
            )
            tag_row = self.db.fetchone("SELECT id FROM tags WHERE name = %s", (tag_name,))
            if tag_row:
                self.db.execute(
                    "INSERT INTO work_tags (work_id, tag_id) VALUES (%s, %s) ON CONFLICT DO NOTHING",
                    (work_id, tag_row[0])
                )
```

> [!IMPORTANT]
> Verificar se `self.db.execute()` e `self.db.fetchone()` existem na classe `CodiceDatabase` em `worker/db.py`. Se não existirem, precisam ser adicionados como wrappers simples para `psycopg2`:
> ```python
> def execute(self, query, params=None):
>     with psycopg2.connect(self.db_url) as conn:
>         with conn.cursor() as cur:
>             cur.execute(query, params)
>
> def fetchone(self, query, params=None):
>     with psycopg2.connect(self.db_url) as conn:
>         with conn.cursor() as cur:
>             cur.execute(query, params)
>             return cur.fetchone()
> ```
> ATENÇÃO: O `%s` é o placeholder do psycopg2. **Não usar `$1`** (que é Go/libpq).

**Passo 3: Corrigir o acesso ao download_cover no `worker/main.py` L151**

Atualmente acessa atributo privado:
```python
local_cover = provider_registry._providers['default'][0].download_cover(...)
```

Adicionar um método público no `ProviderRegistry` (`worker/providers/registry.py`):
```python
def download_cover(self, cover_url: str, file_path: str, covers_dir: str) -> str:
    """Download cover image using the first available provider."""
    for provider_list in self._providers.values():
        for provider in provider_list:
            if hasattr(provider, 'download_cover'):
                result = provider.download_cover(cover_url, file_path, covers_dir)
                if result:
                    return result
    return ""
```

E no `main.py` L151, trocar para:
```python
local_cover = provider_registry.download_cover(enriched.cover_url, file_path, covers_dir)
```

### Critérios de Aceite

- [ ] Campos `series`, `isbn`, `language`, `publisher`, `description`, `publication_date` são gravados na tabela `works`
- [ ] Tags são gravadas na tabela `work_tags`
- [ ] Campos com `_lock = TRUE` no banco NÃO são sobrescritos
- [ ] `download_cover` acessado via método público, não atributo privado
- [ ] Testes existentes continuam passando

### O que NÃO fazer

- ❌ NÃO remover o método `update_work_metadata` de `db.py` nesta tarefa (é a Tarefa 7)
- ❌ NÃO mudar os placeholders SQL — `analyzer.py` usa `%s` (psycopg2), não `$1` (Go)
- ❌ NÃO alterar `ExtractedMetadata` em `base.py`

---

## Tarefa 5 (Fix): Upload rejeita formatos suportados

### Motivação

O backend Go aceita upload apenas de `.pdf`, `.epub`, `.cbz`. Mas os extratores Python já suportam `.cbr`, `.txt`, `.md`, `.mp3`, `.m4a`, `.m4b`, `.flac`, `.ogg`, `.wav`. Arquivos nesses formatos são rejeitados antes de chegarem ao worker.

### Arquivos a modificar

| Arquivo | Ação |
|:--------|:-----|
| `backend/internal/handlers/upload.go` L42-46 | **MODIFICAR** — expandir lista de extensões aceitas |
| `backend/internal/handlers/upload.go` L166-169 | **MODIFICAR** — mesma mudança no `HandleBulkImport` |

### Implementação

**Em `upload.go` L42-46**, substituir:

```go
// 3. Validate file extension (PDF, EPUB, or CBZ)
ext := strings.ToLower(filepath.Ext(header.Filename))
if ext != ".pdf" && ext != ".epub" && ext != ".cbz" {
    http.Error(w, "Unsupported file format. Please upload PDF, EPUB, or CBZ only.", http.StatusBadRequest)
    return
}
```

Por:

```go
// 3. Validate file extension against supported formats
ext := strings.ToLower(filepath.Ext(header.Filename))
supportedFormats := map[string]bool{
    ".pdf": true, ".epub": true,
    ".cbz": true, ".cbr": true,
    ".txt": true, ".md": true,
    ".mobi": true, ".azw": true, ".azw3": true,
    ".mp3": true, ".m4a": true, ".m4b": true,
    ".flac": true, ".ogg": true, ".wav": true,
}
if !supportedFormats[ext] {
    http.Error(w, "Unsupported file format", http.StatusBadRequest)
    return
}
```

**Em `upload.go` L166-169** (dentro de `HandleBulkImport`), aplicar a mesma mudança — substituir a checagem hardcoded por um map `supportedFormats`.

> [!TIP]
> Para evitar duplicação, extrair o map para uma variável de pacote:
> ```go
> // No topo do arquivo, após os imports:
> var SupportedFormats = map[string]bool{
>     ".pdf": true, ".epub": true,
>     ".cbz": true, ".cbr": true,
>     ".txt": true, ".md": true,
>     ".mobi": true, ".azw": true, ".azw3": true,
>     ".mp3": true, ".m4a": true, ".m4b": true,
>     ".flac": true, ".ogg": true, ".wav": true,
> }
> ```
> E usar `SupportedFormats[ext]` nos dois lugares.

### Critérios de Aceite

- [ ] Upload aceita: `.pdf`, `.epub`, `.cbz`, `.cbr`, `.txt`, `.md`, `.mobi`, `.azw`, `.azw3`, `.mp3`, `.m4a`, `.m4b`, `.flac`, `.ogg`, `.wav`
- [ ] Bulk import aceita os mesmos formatos
- [ ] Formatos não suportados (ex: `.exe`, `.zip`, `.docx`) são rejeitados
- [ ] Mensagem de erro é genérica ("Unsupported file format") — não listar formatos aceitos no erro

### O que NÃO fazer

- ❌ NÃO alterar o tamanho máximo de upload (50MB) nesta tarefa
- ❌ NÃO adicionar `.mobi`/`.azw`/`.azw3` se a Tarefa 1 (MOBI extractor) ainda não foi feita — o worker não saberia processar. Nesse caso, adicionar só `.cbr`, `.txt`, `.md` e os formatos de áudio.

---

## Tarefa 6 (Fix): CBR page streaming no Go usa `archive/zip`

### Motivação

O handler de páginas em `pages.go` aceita `.cbr` na validação (L62), mas chama `listCBZPages()` e `servePageFromZip()` que usam `archive/zip`. Arquivos RAR reais (a maioria dos CBRs) falham porque o Go stdlib não lê RAR.

No worker Python, o `cbr_extractor.py` já usa `rarfile` corretamente. O problema é só no streaming de páginas do backend Go.

### Abordagem recomendada: extração para diretório temporário

Em vez de adicionar uma lib RAR ao Go (as opções são limitadas — `nwaples/rardecode` só suporta RAR v3, não v5), a abordagem mais robusta é:

1. **Primeira vez que um CBR é acessado**: Extrair imagens para um diretório cache
2. **Acessos subsequentes**: Servir direto do cache
3. O cache fica em `{CODICE_STORAGE_PATH}/cache/pages/{work_id}/`

Esta é a mesma abordagem que o Komga usa para formatos que não suportam random access.

### Arquivos a modificar

| Arquivo | Ação |
|:--------|:-----|
| `backend/internal/handlers/pages.go` | **MODIFICAR** — separar lógica de ZIP e diretório, adicionar cache extraction |
| `backend/go.mod` | **POSSIVELMENTE MODIFICAR** — se usar lib RAR em Go |

### Implementação passo a passo

**Passo 1: Modificar `GetPages` para rotear por extensão**

Em `pages.go` L60-72, onde atualmente chama `listCBZPages` para ambos CBZ e CBR:

```go
ext := strings.ToLower(filepath.Ext(fullPath))

var pages []PageInfo
var err error

switch ext {
case ".cbz":
    pages, err = listCBZPages(fullPath)
case ".cbr":
    pages, err = listCachedPages(fullPath, id, storagePath)
default:
    http.Error(w, "Format does not support page listing", http.StatusBadRequest)
    return
}
```

**Passo 2: Implementar `listCachedPages` e `ensureCBRExtracted`**

```go
func listCachedPages(rarPath string, workID string, storagePath string) ([]PageInfo, error) {
    cacheDir := filepath.Join(storagePath, "cache", "pages", workID)

    // If cache doesn't exist, extract via external unrar command
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        if err := extractCBR(rarPath, cacheDir); err != nil {
            return nil, fmt.Errorf("failed to extract CBR: %w", err)
        }
    }

    // List images from cache directory
    validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
    var images []string

    filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return nil
        }
        ext := strings.ToLower(filepath.Ext(info.Name()))
        if validExts[ext] {
            images = append(images, info.Name())
        }
        return nil
    })

    sort.Strings(images)

    pages := make([]PageInfo, 0, len(images))
    for i, name := range images {
        pages = append(pages, PageInfo{
            Number:   i,
            FileName: name,
            URL:      fmt.Sprintf("/pages/%d", i),
        })
    }

    return pages, nil
}

func extractCBR(rarPath string, cacheDir string) error {
    os.MkdirAll(cacheDir, 0755)

    // Use system unrar command (available in most Docker images via apt/apk)
    cmd := exec.Command("unrar", "e", "-o+", rarPath, cacheDir)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("unrar failed: %s - %w", string(output), err)
    }

    // Remove non-image files that may have been extracted (ComicInfo.xml, etc.)
    validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
    filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return nil
        }
        ext := strings.ToLower(filepath.Ext(info.Name()))
        if !validExts[ext] {
            os.Remove(path)
        }
        return nil
    })

    return nil
}
```

**Passo 3: Modificar `ServePage` para CBR**

Em `ServePage` (~L107), precisa rotear entre ZIP direto e cache:

```go
ext := strings.ToLower(filepath.Ext(fullPath))
switch ext {
case ".cbz":
    servePageFromZip(w, r, fullPath, pageNum)
case ".cbr":
    servePageFromCache(w, r, id, storagePath, pageNum)
default:
    http.Error(w, "Format does not support page serving", http.StatusBadRequest)
}
```

```go
func servePageFromCache(w http.ResponseWriter, r *http.Request, workID string, storagePath string, pageNum int) {
    cacheDir := filepath.Join(storagePath, "cache", "pages", workID)

    // Ensure extracted
    // (listCachedPages would have extracted on first GetPages call,
    //  but handle the case where ServePage is called directly)
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        http.Error(w, "Pages not yet extracted. Call GetPages first.", http.StatusNotFound)
        return
    }

    // List and sort images
    validExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
    var images []string
    filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() { return nil }
        ext := strings.ToLower(filepath.Ext(info.Name()))
        if validExts[ext] { images = append(images, info.Name()) }
        return nil
    })
    sort.Strings(images)

    if pageNum < 0 || pageNum >= len(images) {
        http.Error(w, "Page not found", http.StatusNotFound)
        return
    }

    pagePath := filepath.Join(cacheDir, images[pageNum])
    http.ServeFile(w, r, pagePath)
}
```

**Passo 4: Dockerfile do backend — instalar `unrar`**

No `backend/Dockerfile`, adicionar o pacote `unrar`:
```dockerfile
# Se Alpine:
RUN apk add --no-cache unrar
# Se Debian/Ubuntu:
RUN apt-get update && apt-get install -y unrar-free && rm -rf /var/lib/apt/lists/*
```

**Passo 5: Adicionar import `os/exec`** ao `pages.go` se não existir.

### Critérios de Aceite

- [ ] CBR com formato RAR real abre e mostra páginas no viewer
- [ ] Páginas são cacheadas em `{CODICE_STORAGE_PATH}/cache/pages/{work_id}/`
- [ ] Acessos subsequentes não re-extraem (usam cache)
- [ ] CBZ continua funcionando via `archive/zip` (sem regressão)
- [ ] `unrar` instalado no Dockerfile

### O que NÃO fazer

- ❌ NÃO tentar parsear RAR em Go puro — as libs existentes (`nwaples/rardecode`) só suportam RAR v3, não RAR v5 que é o padrão atual
- ❌ NÃO extrair o CBR inteiro para memória — usar disco (cache dir)
- ❌ NÃO limpar o cache automaticamente por enquanto — isso é feature futura (LRU cache com TTL)

---

## Tarefa 7 (Fix): Limpar `db.py` — consolidar com `analyzer.py`

### Motivação

O arquivo `worker/db.py` contém a classe `CodiceDatabase` com o método `update_work_metadata` — este é o código **antigo** (pré-refatoração). O `worker/main.py` atual usa `analyzer.save_metadata` para salvar metadados. Mas `CodiceDatabase` ainda é importada em `main.py` L14 e instanciada em L83, e é passada ao `Analyzer` como dependência.

O problema é que existem **dois caminhos de salvamento** — `db.update_work_metadata` (antigo, não usado) e `analyzer.save_metadata` (novo, usado). Isso confunde qualquer desenvolvedor que leia o código.

### O que fazer

1. **Verificar** se `CodiceDatabase.update_work_metadata` é chamado em algum lugar (grep por `update_work_metadata`). Se não → pode ser removido.

2. **Manter** `CodiceDatabase` como classe de conexão/wrapper, mas remover o método `update_work_metadata` que é código morto.

3. **Verificar** se `analyzer.py` usa `self.db.execute()` e `self.db.fetchone()`. Se `CodiceDatabase` não tem esses métodos, adicioná-los.

4. **Resultado final**: `db.py` fica como um wrapper limpo de conexão PostgreSQL. `analyzer.py` contém toda a lógica de salvamento.

### Implementação

**Passo 1: Grep para verificar uso**

```bash
grep -rn "update_work_metadata" worker/
grep -rn "CodiceDatabase" worker/
```

**Passo 2: Reescrever `worker/db.py`** como wrapper limpo:

```python
import os
import psycopg2


class CodiceDatabase:
    """PostgreSQL connection wrapper for the Códice worker."""

    def __init__(self):
        self.db_url = os.getenv(
            "DATABASE_URL",
            "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
        )

    def execute(self, query, params=None):
        """Execute a query (INSERT, UPDATE, DELETE)."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)

    def fetchone(self, query, params=None):
        """Execute a query and return one row."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)
                return cur.fetchone()

    def fetchall(self, query, params=None):
        """Execute a query and return all rows."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)
                return cur.fetchall()
```

### Critérios de Aceite

- [ ] `update_work_metadata` removido (código morto)
- [ ] `CodiceDatabase` tem métodos `execute`, `fetchone`, `fetchall`
- [ ] `analyzer.py` funciona com o novo `CodiceDatabase`
- [ ] Nenhum import quebrado

### O que NÃO fazer

- ❌ NÃO remover a classe `CodiceDatabase` inteira — ela é usada pelo `Analyzer`
- ❌ NÃO criar uma nova classe — refatorar a existente

---

## Ordem Recomendada de Execução

> [!IMPORTANT]
> As tarefas F (fixes) devem ser feitas **antes** das tarefas 1-3, pois corrigem problemas que afetam o sistema inteiro.

1. **Tarefa 7** (limpar db.py) — 30 min, pré-requisito para a Tarefa 4 funcionar corretamente
2. **Tarefa 4** (fix analyzer.save_metadata) — 1h, **mais crítico** — sem isso, providers são inúteis
3. **Tarefa 5** (fix upload formats) — 15 min, trivial
4. **Tarefa 2** (lxml no epub) — 30 min, baixo risco
5. **Tarefa 6** (CBR page streaming) — 2-3h, requer `unrar` no Docker
6. **Tarefa 3** (migration 009) — 1h, SQL + worker
7. **Tarefa 1** (MOBI extractor) — 4-6h, mais complexa, fazer por último
