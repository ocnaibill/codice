// The fields of a work a provider may suggest something for, as a reader names them, in the order they are listed.
export const FIELD_LABELS = {
  title: 'Título',
  author: 'Autor',
  contributors: 'Outras pessoas',
  series: 'Série',
  series_index: 'Número na série',
  isbn: 'ISBN',
  language: 'Idioma',
  publisher: 'Editora',
  publication_date: 'Data de publicação',
  original_year: 'Ano da primeira publicação',
  description: 'Sinopse',
  tags: 'Etiquetas',
  // What a provider says of the series the work is in, not of the work (DEC-171): it goes to the series when accepted.
  series_status: 'Situação da série',
  series_original_title: 'Título original da série',
};
export const FIELD_ORDER = Object.keys(FIELD_LABELS);
