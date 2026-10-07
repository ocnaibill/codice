# Arquivos hostis: o que o Códice faz com o que está errado por dentro (07/10/2026)

Item 1 do #89 (os arquivos de verdade), a parte que **não depende de arquivos reais**: arquivos sintéticos que passam na porta (extensão, primeiros bytes) e estão errados por dentro, e arquivos legítimos que um filtro descuidado recusaria. O que um Códice saudável faz: **recusar com uma mensagem clara**, ou **aceitar e terminar limpo** (texto `failed` ou `empty`, com o motivo), sem derrubar o worker, sem deixar obra pela metade e **sem recusar o que é legítimo**.

**Como repetir:** `worker/venv/bin/python testdata/generate_hostile.py --out DIR` gera 36 arquivos e um `manifest.json` com o que se espera de cada um; `testdata/run_hostile.py` envia todos a uma pilha **descartável** (ele sobe dezenas de arquivos, alguns quebrados de propósito) e diz PASS, REVIEW ou FAIL para cada um. Rodado numa pilha limpa construída da `main` (`c184788`).

**Resultado da primeira rodada: 26 PASS, 4 REVIEW, 6 FAIL**, o worker e o backend sem reiniciar, a fila vazia ao fim, e **nada escapou da pasta** (os ZIP com `../` e caminho absoluto foram lidos sem escrever fora).

## O que passou

- **PDF:** truncado, só com cabeçalho e sem páginas terminam em `failed`/`empty` com motivo; **xref quebrada é reconstruída e o texto sai**; **senha para abrir** termina `failed` ("document closed or encrypted"); **senha só de dono** (copiar proibido) **abre e o texto sai**; PDF chamado `.epub` e EPUB chamado `.pdf` são **recusados** na porta.
- **EPUB:** sem OPF termina `failed` ("the package file cannot be read"); **XHTML malformado é lido**; **fontes ofuscadas (comuns em livros de editoras) não são tomadas por DRM**; sem título nem autor vira obra com título do arquivo; **bomba de descompressão** (512 MiB em ~780 KB) termina `failed` ("the EPUB expands into too much data"), o worker segue.
- **CBZ:** imagem truncada, **imagem que declara 60.000 × 60.000 px**, nomes em japonês e acentos: as obras abrem e o worker não aloca o que a imagem promete; um ZIP só com PDF é recusado.
- **Áudio:** MP3 com ID3 que declara 200 MB e acaba, MP3 truncado e M4B só com `ftyp` terminam `failed` com o motivo.
- **TXT binário** com extensão `.txt` é recusado; TXT com BOM UTF-8 é lido.

## O que falhou (cada um virou issue, e todas foram corrigidas: veja "Depois das correções")

1. **EPUB com DRM é aceito e indexado como se fosse texto.** O capítulo cifrado foi lido como texto: **1 trecho de bytes sem sentido, estado `ready`**. Numa biblioteca com EPUBs da Kindle/Adobe, o texto de lixo entra na busca e a obra parece normal até alguém abrir. O #89 já diz o que se espera: "recusa explicada, nada de tentativa de quebra". O sinal está no `META-INF/encryption.xml`: algoritmo que **não** seja a ofuscação de fontes (`http://www.idpf.org/2008/embedding` e `http://ns.adobe.com/pdf/enc#RC`).
2. **Nome de arquivo comprido, com quebra de linha ou com NUL dá `500 Error saving file`.** O nome enviado entra cru no nome guardado (`<hex>_<nome>`) e no título: 300 caracteres, ou **um nome de 250 bytes de um disco real com acentos, que passa dos 255 bytes do sistema de arquivos depois do prefixo**, quebram o `rename`. Vale também para a importação em lote de pastas. Esperado: nome limpo (sem controles, NUL e marcas de direção), cortado para caber, com a extensão preservada.
3. **Título de mais de 255 caracteres derruba a ingestão com um erro técnico** (`StringDataRightTruncation: value too long for type character varying(255)`). Há EPUBs reais que põem a sinopse inteira em `dc:title`. Esperado: o campo é cortado no limite da coluna (título e edição 255, editora 256, série 512, nomes 255).
4. **PDF com página gigante (14.400 × 14.400 pt, o máximo do formato) falha a ingestão inteira** em `FzErrorLimit: Overly large image`, na hora de gerar a capa; a obra existe, mas sem texto e sem capa, por uma coisa opcional. Esperado: a capa é gerada em tamanho limitado (ou pulada) e a ingestão segue.
5. **TXT e MD em Windows-1252 e TXT em UTF-16 com BOM são recusados** com "not a valid .txt file (not UTF-8 text)": é o TXT antigo e o "Unicode" do Bloco de Notas. **Decisão do mantenedor**: aceitar e converter (o leitor e a extração também precisam ler a codificação) ou manter a recusa e dizer em português como converter. Não é um defeito de segurança.

## Nota

O que este relatório **não** prova: são arquivos inventados. Os itens do #89 continuam sem marca até haver arquivos reais (DRM da Kindle de verdade, M4B de audiolivro, RAR5 de HQ).

## Depois das correções (mesmo dia)

Cada achado virou um PR pequeno, verificado numa pilha descartável: **#168** nomes de arquivo (#173), **#171** texto em Windows-1252 e UTF-16 levado a UTF-8 com aviso ao dono (#174, DEC-126), **#169** campos longos demais (#175), **#170** capa de PDF limitada e opcional (#176), **#167** EPUB com DRM recusado com o motivo (#177). Rodando de novo os 36 arquivos numa pilha limpa da `main`: **36 de 36 como se espera**.

Três linhas do gerador foram acertadas, e não eram defeitos do Códice: o **TXT em UTF-16 tinha o mesmo texto do de Windows-1252**, e o mesmo texto em duas codificações é a mesma obra depois de convertido (duplicata, como diz o DEC-126), então ganhou um texto próprio; e os **dois nomes com NUL e quebra de linha crus** não são um formulário válido (o cabeçalho não aceita), e a resposta certa é **400 com mensagem**, não aceitar (a limpeza do nome cobre os que vêm de um disco, na importação em lote).
