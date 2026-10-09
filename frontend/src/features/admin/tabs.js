// The tabs of the administration, in the order they are shown: the key (which is also what the address says, #182) and the name.
export const ADMIN_TABS = [
  ['jobs', 'Trabalhos'],
  ['storage', 'Armazenamento'],
  ['system', 'Sistema'],
  ['trash', 'Lixeira'],
  ['suggestions', 'Sugestões'],
  ['providers', 'Provedores'],
  ['categories', 'Categorias'],
  ['duplicates', 'Duplicatas'],
  ['ocr', 'OCR'],
  ['dictionaries', 'Dicionários'],
  ['accounts', 'Contas'],
  ['logins', 'Entradas'],
];

// The tabs only the owner has.
export const OWNER_TABS = [
  ['embeddings', 'IA local'],
  ['ldap', 'Login externo'],
];

/** The tab the administration opens on. */
export const FIRST_ADMIN_TAB = 'jobs';

/** Every key of a tab, the owner's included: what an address may say. */
export const ADMIN_TAB_KEYS = [...ADMIN_TABS, ...OWNER_TABS].map(([key]) => key);
