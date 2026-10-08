/** "1 obra", "12 obras". */
export const worksText = (n) => `${n} ${n === 1 ? 'obra' : 'obras'}`;

/** What a card of a collection says of what is in it and of how much the person read: "12 obras · Leu 3 de 12". */
export function collectionLine({ workCount = 0, completedCount = 0 }) {
  if (workCount === 0) return 'Nenhuma obra ainda';
  return completedCount > 0 ? `${worksText(workCount)} · Leu ${completedCount} de ${workCount}` : worksText(workCount);
}

/**
 * The words that change with what is on screen: an official collection of the library, or a list of the person (#207). The
 * management of one is the staff's and of the other is the person's, and what happens to the works is not the same.
 */
export function wordsOf(kind) {
  if (kind === 'personal') {
    return {
      thing: 'lista',
      eyebrow: 'Biblioteca / Lista',
      dialog: 'Lista',
      nowhere: 'Esta lista ainda não tem obras.',
      retired: 'Lista aposentada',
      retiredOnes: 'Listas aposentadas',
      renameNote: 'Só o nome da lista muda: as obras continuam como estão.',
      retireNote: 'As obras continuam no acervo: a lista só sai do menu, e dá para restaurá-la na lista das aposentadas.',
      removeNote: 'A obra continua no acervo: só sai da lista. Dá para acrescentá-la de novo.',
      added: 'foi para o fim da lista.',
      restoreHint: 'As obras voltam quando a lista for restaurada.',
    };
  }
  return {
    thing: 'coleção',
    eyebrow: 'Biblioteca / Coleção',
    dialog: 'Coleção',
    nowhere: 'Esta coleção ainda não tem obras.',
    retired: 'Coleção aposentada',
    retiredOnes: 'Coleções aposentadas',
    renameNote: 'As obras da coleção passam a ter esse nome como série. O nome antigo continua levando a ela.',
    retireNote: 'As obras não são apagadas nem mudam de série: a coleção só sai do acervo, e dá para restaurá-la na lista das aposentadas.',
    removeNote: 'A série da obra é limpa e travada, para a análise do arquivo não recolocá-la. Dá para acrescentá-la de novo.',
    added: 'foi para o fim da coleção.',
    restoreHint: 'As obras voltam quando a coleção for restaurada.',
  };
}
