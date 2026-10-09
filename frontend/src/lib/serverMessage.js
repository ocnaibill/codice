// What the server says when it refuses something, as a person reads it (#77). The server answers in plain text, in English
// for the most part (its API is for programs too), and the screens used to show that text as it came. Here what a person can
// bring about is said in Portuguese, and what is not known is never shown in English: the screen's own sentence is.

// Removing a folder is refused while the catalog keeps files in it: the ones of works in the catalog are moved to the managed storage,
// and the ones of retired works go away with those works, which are deleted for good (#230). The sentence says which way out there is.
function stillCatalogued(active, retired) {
  const files = (n) => `${n} arquivo${n === 1 ? '' : 's'}`;
  if (active > 0 && retired > 0) {
    return `Ainda há ${files(active)} desta pasta no acervo e ${files(retired)} de obras retiradas. Mova os do acervo para o armazenamento gerenciado e apague de vez as obras retiradas, pelo botão desta pasta ou pela Lixeira.`;
  }
  if (retired > 0) {
    return `Só restam ${files(retired)} de obras retiradas. Apague de vez essas obras, pelo botão desta pasta ou pela Lixeira: os arquivos da pasta continuam onde estão.`;
  }
  return 'Ainda há arquivos desta pasta no acervo: mova-os para o armazenamento gerenciado antes.';
}

const EXACT = {
  // Entering, and the accounts
  'Invalid username or password': 'Usuário ou senha incorretos.',
  'Registration is disabled': 'O cadastro está desativado neste servidor. Peça um convite a quem cuida do acervo.',
  'Registration is disabled in production': 'O cadastro está desativado neste servidor. Peça um convite a quem cuida do acervo.',
  'Username and password are required': 'Informe o usuário e a senha.',
  'Username and password are required for master setup': 'Informe o usuário e a senha.',
  'Error creating user. Username or email already exists.': 'Esse usuário ou e-mail já está em uso.',
  'That username or email is already taken': 'Esse usuário ou e-mail já está em uso.',
  'A username of up to 50 characters is required': 'Informe um usuário de até 50 caracteres.',
  'The email address is not valid': 'O endereço de e-mail não é válido.',
  'First-time setup has already been completed': 'A configuração inicial já foi feita. Entre com a sua conta.',
  'Forbidden: sign in to change the password': 'Entre na sua conta para trocar a senha.',
  'The new password must have at least 8 characters': 'A senha precisa ter pelo menos 8 caracteres.',
  'The password must have at least 8 characters': 'A senha precisa ter pelo menos 8 caracteres.',
  'This account has no local password': 'Esta conta não tem senha local: ela entra pelo servidor de login.',
  'The current password is not correct': 'A senha atual não está correta.',
  'The password is not correct': 'A senha não está correta.',
  // The owner's backup buttons (DEC-123)
  'The password is required': 'Digite a sua senha.',
  // "Meus dados"
  'Export not found': 'Esse arquivo não existe mais.',
  'The export is not ready': 'O arquivo ainda não está pronto, ou já expirou.',
  'Too many exports: try again in a while': 'Você já pediu o arquivo 3 vezes na última hora. Tente de novo mais tarde.',
  'Session not found': 'Essa sessão não existe mais: talvez já tenha sido encerrada.',
  'Backups from the panel are not set up': 'O backup pelo painel ainda não foi configurado no servidor.',
  'backups from the panel are not set up: CODICE_BACKUP_DIR and CODICE_BACKUP_PASSPHRASE_FILE are needed': 'Para fazer backups pelo painel, o servidor precisa de CODICE_BACKUP_DIR (a pasta) e CODICE_BACKUP_PASSPHRASE_FILE (o arquivo com a frase de segurança).',
  'That is not a package in the backup folder': 'Esse pacote não está na pasta de backups.',
  'that is not a package in the backup folder': 'Esse pacote não está na pasta de backups.',
  'the passphrase file is missing, unreadable or shorter than 8 characters': 'O arquivo com a frase de segurança não existe, não pode ser lido ou tem menos de 8 caracteres.',
  'This invitation is for another email address': 'Este convite é para outro endereço de e-mail.',
  'The invitation is no longer pending': 'Este convite não está mais pendente.',
  'Invitation not found': 'Convite não encontrado.',
  'This account cannot be reset from here': 'Esta conta não pode ter a senha redefinida por aqui.',
  'LDAP is not configured on the server, so accounts cannot be created from it': 'O login externo (LDAP) não está configurado neste servidor, então não dá para criar contas a partir dele.',
  "role must be 'admin' or 'reader'": 'O papel é administrador ou leitor.',
  "confirmUsername must repeat the account's username": 'Digite o nome de usuário da conta para confirmar.',
  'Forbidden': 'Você não tem permissão para isso.',
  'Forbidden: administrator role required': 'Só quem administra o acervo pode fazer isso.',
  'Forbidden: owner role required': 'Só o dono do acervo pode fazer isso.',
  'Access denied: Authentication required': 'Entre na sua conta para continuar.',
  'User not found': 'Conta não encontrada.',
  'Token not found': 'Aplicativo não encontrado.',
  'Request not found': 'Pedido não encontrado.',
  'The request is no longer waiting for a decision': 'Este pedido não espera mais uma decisão.',
  'Notice not found': 'Aviso não encontrado.',
  'Person not found': 'Pessoa não encontrada.',
  'name is required (up to 100 characters)': 'Dê um nome (até 100 caracteres).',
  'family and given are the parts of the name': 'Informe o sobrenome e o nome.',
  // What the domain says when it refuses (the errors of the packages the handlers read out)
  'a retired work cannot be joined: restore it first': 'Uma obra retirada não pode ser juntada: restaure-a antes.',
  'a work cannot be joined to itself': 'Uma obra não pode ser juntada a ela mesma.',
  'both works must be active': 'As duas obras precisam estar ativas.',
  'the only edition of a work cannot be separated from it': 'A única edição de uma obra não pode ser separada dela.',
  'keep must be one of the two works': 'Escolha uma das duas obras para ficar.',
  'keep must be one of the two people of the pair': 'Escolha uma das duas pessoas do par para ficar.',
  'family and given must be made of the words of the name': 'O sobrenome e o nome precisam ser feitos das palavras do nome.',
  'no such pending candidate': 'Essa sugestão não está mais pendente.',
  'candidate not found': 'Sugestão não encontrada.',
  'a transfer is already waiting for an answer': 'Já há uma transferência esperando resposta.',
  'there is no transfer waiting for an answer': 'Não há transferência esperando resposta.',
  'that account cannot receive ownership: it must be active, have a local password and no linked external identity': 'Essa conta não pode receber a titularidade: precisa estar ativa, ter senha local e nenhuma identidade externa ligada.',
  "the former owner becomes 'admin' or 'reader'": 'O dono atual passa a ser administrador ou leitor.',
  'the instance has no owner': 'Este acervo não tem dono.',
  'account not found': 'Conta não encontrada.',
  'the local password is not correct': 'A senha local não está correta.',
  'that username is already taken': 'Esse usuário já está em uso.',
  'that account cannot be linked to a directory identity': 'Essa conta não pode ser ligada a uma identidade do diretório.',
  'that identity is already linked': 'Essa identidade já está ligada a uma conta.',
  'the directory is unavailable': 'O servidor de login (diretório) não está respondendo.',
  'the directory refused the credentials': 'O servidor de login (diretório) recusou o usuário ou a senha.',
  'no such entry in the directory': 'Esse usuário não existe no diretório.',
  'the ticket is not valid': 'O pedido de ligação não vale mais. Entre de novo.',
  'no such item in the trash': 'Esse item não está na lixeira.',
  'only available managed files can be trashed': 'Só arquivos disponíveis do armazenamento gerenciado podem ir para a lixeira.',
  'job is not in a state that allows this': 'Este trabalho não está num estado que permita isso.',
  'job not found': 'Trabalho não encontrado.',
  'file not found': 'Arquivo não encontrado.',
  'work or edition not found': 'Obra ou edição não encontrada.',
  'the plan changed since the preview; preview again': 'O acervo mudou desde que o plano foi mostrado. Veja o plano de novo.',
  'the provider needs an API key that is not configured': 'Este provedor precisa de uma chave de API que não está configurada.',
  'unknown metadata provider': 'Provedor não encontrado.',
  'unsupported file format': 'Esse formato de arquivo não é aceito.',
  'the original file changed since it was catalogued': 'O arquivo original mudou desde que foi catalogado.',
  'the original file is missing': 'O arquivo original não está mais lá.',
  'the file cannot be moved': 'Não foi possível mover o arquivo.',
  'the file is not in a referenced location': 'O arquivo não está numa pasta de referência.',
  'the destination is already taken': 'Já existe um arquivo nesse destino.',
  'the path is not a safe relative path': 'O caminho não é um caminho relativo seguro.',
  'only the owner can do this': 'Só o dono do acervo pode fazer isso.',
  'invalid credentials': 'Usuário ou senha incorretos.',
  'invalid session': 'Sua sessão acabou. Entre de novo.',
  // Notes
  'A bookmark needs a place': 'Um marcador precisa de um lugar no arquivo.',
  'A note cannot be left empty': 'Uma anotação não pode ficar vazia.',
  'Quote text is required': 'Escreva o trecho ou a anotação.',
  'A place needs the file it is in': 'Um lugar precisa do arquivo em que está.',
  "That file is not one of this book's files": 'Esse arquivo não é um dos arquivos desta obra.',
  'color is terracotta, sepia, sage or indigo': 'Escolha uma das quatro cores.',
  'Note not found': 'Anotação não encontrada.',
  'Concept not found': 'Conceito não encontrado.',
  'Relation not found': 'Relação não encontrada.',
  // The library
  'Book not found': 'Obra não encontrada.',
  'Collection not found': 'Coleção não encontrada.',
  'Title not found': 'Esse título não está mais na obra.',
  'Edition not found': 'Essa edição não está mais na obra.',
  'Contributor not found': 'Essa pessoa não está mais creditada nessa função.',
  'Work not found': 'Obra não encontrada.',
  'Book or file not found': 'Obra ou arquivo não encontrado.',
  'Work or edition not found': 'Obra ou edição não encontrada.',
  'File not found': 'Arquivo não encontrado.',
  'Work has no file': 'Esta obra não tem arquivo.',
  'Candidate not found': 'Sugestão não encontrada.',
  'Item not found': 'Item não encontrado.',
  'Page not found': 'Página não encontrada.',
  'Title is required': 'Informe o título.',
  'Retire the book before deleting it permanently': 'Retire a obra antes de apagá-la de vez.',
  'That is not a file of the same work': 'Esse arquivo não é da mesma obra.',
  'Unsupported file format': 'Esse formato de arquivo não é aceito.',
  'File is too large': 'O arquivo é grande demais.',
  'Error reading uploaded file': 'Não foi possível ler o arquivo enviado.',
  'Candidate contributors are malformed': 'A sugestão tem um valor que não dá para aceitar.',
  'Candidate tags are malformed': 'A sugestão tem um valor que não dá para aceitar.',
  'Candidate value is not a number': 'A sugestão tem um valor que não dá para aceitar.',
  'Unsupported candidate field': 'A sugestão tem um valor que não dá para aceitar.',
  'Job not found': 'Trabalho não encontrado.',
  'The job is not in a state that allows this': 'Este trabalho não está num estado que permita isso.',
  'Provider not found': 'Provedor não encontrado.',
  'This provider needs an API key and the worker has none: set it in the environment of the worker': 'Este provedor precisa de uma chave de API e o worker não tem nenhuma: defina-a no ambiente do worker.',
  'revalidateHours must be between 1 and 336': 'O intervalo de revalidação vai de 1 a 336 horas.',
  // Search
  'Search service unavailable': 'A busca não está disponível agora.',
  'The search is too long': 'A busca é longa demais.',
  'the search is too long': 'A busca é longa demais.',
  'Missing search query': 'Digite o que buscar.',
  "Missing 'q' parameter": 'Digite o que buscar.',
  // Folders and storage
  'A path is required': 'Informe a pasta.',
  'The directory does not exist or cannot be read': 'A pasta não existe ou não pode ser lida.',
  'The directory overlaps the managed storage': 'A pasta se sobrepõe ao armazenamento gerenciado.',
  'The path is not a directory': 'O caminho não é uma pasta.',
  'The path must be absolute': 'O caminho precisa ser absoluto (começar por /).',
  'The subdirectory is not a safe relative path': 'A subpasta não é um caminho relativo seguro.',
  'The directory is already a root': 'Essa pasta já está autorizada.',
  'The directory overlaps an existing root': 'Essa pasta se sobrepõe a outra já autorizada.',
  'Forbidden: directory is outside the allowed import roots': 'Essa pasta está fora das pastas autorizadas.',
  'Directory not found': 'Pasta não encontrada.',
  // The categories (DEC-140)
  'A category with that name already exists there': 'Já existe uma categoria com esse nome nesse lugar.',
  'A category needs a name of up to 80 characters': 'Dê à categoria um nome de até 80 caracteres.',
  'Categories go at most 3 levels deep': 'As categorias vão até três níveis: a categoria, a subcategoria e a de baixo.',
  'A category cannot go inside itself': 'Uma categoria não pode ficar dentro dela mesma, nem do que há nela.',
  'Move or delete its subcategories first': 'Mova ou apague as subcategorias primeiro.',
  'Category not found': 'Categoria não encontrada.',
  'Root not found': 'Pasta não encontrada.',
};

const PATTERNS = [
  [/^Files in this directory are still catalogued: (\d+) in the catalog, (\d+) from retired works$/, (m) => stillCatalogued(Number(m[1]), Number(m[2]))],
  [/^The note is longer than (\d+) characters$/, (m) => `A anotação passa de ${m[1]} caracteres.`],
  [/^a tag has at most (\d+) characters$/, (m) => `Uma tag tem no máximo ${m[1]} caracteres.`],
  [/^at most (\d+) tags$/, (m) => `No máximo ${m[1]} tags.`],
  [/^text is not valid UTF-8$/, () => 'O texto tem caracteres que não dá para guardar.'],
  [/^invalid locator\b/, () => 'O lugar indicado não existe neste arquivo.'],
];

// English where nothing says Portuguese: the words that only English has, and not one of the Portuguese ("a", "no", "me").
const ENGLISH = /\b(?:the|is|are|was|were|must|cannot|invalid|required|missing|expected|needs|does|not|has|have|with|from|that|this|it|be|at least|at most|already|error|failed|unable|forbidden|unauthorized)\b/i;
const PORTUGUESE = /[áàâãéêíóôõúç]|\b(?:não|você|para|uma|obra|arquivo|senha|conta|nenhum|nenhuma|informe|precisa|esta|este|esse|essa|desta|deste|está|são|foi|ainda)\b/i;

/** The text of a refusal that a person can read: said in Portuguese, or the fallback when it is only English that nobody translated. */
export function messageOf(text) {
  const value = typeof text === 'string' ? text.trim() : '';
  if (!value) return '';
  if (EXACT[value]) return EXACT[value];
  for (const [pattern, say] of PATTERNS) {
    const match = value.match(pattern);
    if (match) return say(match);
  }
  if (PORTUGUESE.test(value)) return value;
  return ENGLISH.test(value) ? '' : value;
}

/** What to tell the person of a failed request: what the server said of it, in Portuguese, when it said something they can act on;
 *  otherwise `fallback`. A failure of the server itself (5xx) is never read out: it says nothing a person can do. */
export function serverMessage(error, fallback) {
  const response = error?.response;
  if (!response || response.status >= 500) return fallback;
  return messageOf(response.data) || fallback;
}
