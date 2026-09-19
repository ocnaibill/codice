"""The analysis of one work: extract what the file says, then ask external
providers for suggestions. Kept apart from the queue loop so it can be tested
and reused by whatever runs the jobs."""
import os

from analyzer import Analyzer


def ensure_file(file_path):
    """Fail early, and permanently, when there is nothing to read. The extractors
    are lenient (a missing or truncated file still "extracts" a title taken from
    its name), so without this check a vanished file would look like a success."""
    if not file_path:
        raise ValueError("the job has no file path")
    if not os.path.isfile(file_path):
        raise FileNotFoundError(f"file not found: {os.path.basename(file_path)}")


def analyze_file(work_id, file_path, extractor, analyzer: Analyzer, provider_registry, covers_dir,
                 checkpoint=lambda: None):
    """Analyze one file and store the results. Returns the extracted metadata.

    - What the file itself says is saved as native metadata: it fills empty
      fields, never overwrites a locked or confirmed one, and records that it
      came from the file.
    checkpoint() is called between steps and raises when the job was cancelled or
    lost, so the work stops at a safe point: nothing is written before the file
    has been read, and each later step writes complete data.

    - External providers only *suggest*. Their data is stored as candidates for
      an admin to accept or reject (DEC-019, DEC-026); it never changes the work
      by itself. The only thing taken from a provider directly is a cover, and
      only when the file has none.
    """
    metadata = extractor.extract(file_path, covers_dir)
    print(f"   📄 Local metadata: {metadata.title} ({metadata.page_count} pages)")
    checkpoint()

    native = {
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
        'raw': metadata.raw,
    }
    analyzer.save_metadata(work_id, native)
    checkpoint()

    enriched = provider_registry.search_best(metadata.title, metadata.format)
    identifiers = dict(native)
    if enriched:
        source = getattr(enriched, 'source', '') or 'provider'
        record = {
            'title': enriched.title, 'author': enriched.author, 'series': enriched.series,
            'series_index': enriched.series_index, 'isbn': enriched.isbn,
            'language': enriched.language, 'publisher': enriched.publisher,
            'publication_date': enriched.publication_date, 'description': enriched.description,
            'tags': enriched.tags,
        }
        raw = enriched.raw or {}
        evidence = {k: raw[k] for k in ('google_id', 'openlibrary_id', 'comicvine_id') if raw.get(k)}
        evidence['query'] = metadata.title
        stored = analyzer.save_candidates(work_id, record, source, evidence)
        print(f"   💡 {stored} suggestion(s) from {source} waiting for review")

        # A provider cover is used only when the file has none.
        if enriched.cover_url and not metadata.cover_path:
            local_cover = provider_registry.download_cover(enriched.cover_url, file_path, covers_dir)
            if local_cover:
                analyzer.save_cover(work_id, local_cover, metadata.title or '')
                print(f"   🖼️ Cover downloaded to: {local_cover}")

        # Provider identifiers are facts about the record, kept for later matching.
        identifiers['enriched_source'] = source
        identifiers['raw'] = dict(metadata.raw or {}, **raw)

    checkpoint()
    analyzer.save_identifiers(work_id, identifiers)
    analyzer.save_media_pages(work_id, native)
    return metadata
