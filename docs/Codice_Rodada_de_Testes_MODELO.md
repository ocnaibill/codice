# Relatório de uma rodada de testes reais (modelo)

Modelo para os relatórios do roteiro da [issue #89](https://github.com/ocnaibill/codice/issues/89). **Um arquivo por rodada**, copiado deste, com o nome `docs/Codice_Rodada_AAAA-MM-DD.md`. Os números **não saem da memória**: saem do servidor, com `scripts/relatorio-rodada.sh`. O que não pôde ser testado com um arquivo ou um cliente real fica como **não verificado** (nunca como pronto), e cada falha vira uma issue.

## 1. Os números do servidor

Rode **na máquina que roda a pilha** (o script só lê, não escreve nada), e cole a saída aqui, ou anexe o arquivo:

```bash
scripts/relatorio-rodada.sh --saida /tmp/rodada.md            # só contagens e motivos: pode ser compartilhado
scripts/relatorio-rodada.sh --com-nomes --saida /tmp/nomes.md  # com o título e o arquivo do que falhou: leia antes de compartilhar
```

O primeiro não traz o título de nenhuma obra nem o nome de nenhum arquivo (os caminhos são cortados dos motivos de erro), então dá para colar numa issue. Ele cobre: versão e saúde dos serviços (com reinícios e falta de memória), o acervo por formato, tamanho e idioma, a fila (tempo por tipo de trabalho, o tempo total da importação e **por que os trabalhos falharam**), o texto e o OCR, os metadados e as duplicatas, o tamanho do banco e da biblioteca, memória e CPU dos contêineres, as contas e as entradas.

Faça **uma leitura antes** e **uma depois** da importação (a diferença é o que a rodada custou), e outra se algo cair no meio.

## 2. Quem e onde

| | Máquina | Sistema e navegador | Rede | Quem |
|---|---|---|---|---|
| Servidor | | | | |
| Cliente 1 | | | | |
| Cliente 2 | | | | |

Matriz do beta: desktop em qualquer sistema (Windows, Mac, Linux), celular **só Android**, sem iOS.

## 3. O que foi testado

Uma linha por item do roteiro da #89 (os números abaixo são as seções dela). **Resultado:** `passou`, `falhou` (com a issue) ou `não verificado` (e por quê).

| # | Item | Arquivos usados (quantos, de onde, quanto pesam) | Resultado | Evidência ou issue |
|---|---|---|---|---|
| 1 | Arquivos reais: PDFs, EPUBs, quadrinhos, áudio, texto | | | |
| 2 | Metadados, pessoas e idioma | | | |
| 3 | Texto, busca e retomada | | | |
| 4 | Leitura, progresso e notas | | | |
| 5 | Instalar, operar e recuperar (inclui backup e **restauração medida**) | | | |
| 6 | Pessoas, contas e acesso | | | |
| 7 | Acessibilidade com leitor de tela (VoiceOver, NVDA, TalkBack) | | | |

## 4. O que quebrou

Cada falha vira uma issue (com o arquivo ou o passo que reproduz, sem o conteúdo do acervo). Liste aqui o número e uma linha.

## 5. O que fica como não verificado

O que não deu para testar nesta rodada, e o motivo. Isto vai para o README como "não verificado", e não como feito.

## 6. O que mudou nos números

O que os números reais dizem diferente dos sintéticos do teste de escala (`docs/Codice_Teste_de_Escala_2026-10-04.md`): tempo de importação por obra, tamanho do índice de busca em relação ao texto, tempo de restauração, peso por perfil de máquina.
