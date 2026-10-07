/** "1 obra", "12 obras". */
export const worksText = (n) => `${n} ${n === 1 ? 'obra' : 'obras'}`;

/** What a card of a collection says of what is in it and of how much the person read: "12 obras · Leu 3 de 12". */
export function collectionLine({ workCount = 0, completedCount = 0 }) {
  if (workCount === 0) return 'Nenhuma obra ainda';
  return completedCount > 0 ? `${worksText(workCount)} · Leu ${completedCount} de ${workCount}` : worksText(workCount);
}
