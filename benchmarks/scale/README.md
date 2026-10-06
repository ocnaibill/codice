# Teste de escala

Mede como o Códice responde com um acervo grande (1.000 e 10.000 obras sintéticas): a lista, a busca, as contagens da barra lateral, a ficha, o OPDS, as capas, as telas de administração, o login, e quanto o worker leva para ler tudo. O relatório dos resultados está em [`docs/Codice_Teste_de_Escala_2026-10-04.md`](../../docs/Codice_Teste_de_Escala_2026-10-04.md).

Tudo é sintético e determinístico: `testdata/generate_scale.py` gera EPUB (com capa), CBZ, TXT e PDF, todos diferentes, com título, autor, série, idioma e editora de conjuntos de tamanho realista, e um texto de vocabulário fixo (uma busca por palavra comum acha milhares de obras; a "marca" de cada obra, `zq000042x`, acha exatamente uma). **Os arquivos são pequenos (~9 KiB):** isto mede a **quantidade** de obras, não o tamanho dos arquivos.

## Como rodar

Use uma pilha **descartável** (projeto, porta e sub-rede próprios; nunca a sua instância). A pasta gerada é montada só para leitura no `backend`, e a importação em massa a lê:

```bash
S=/caminho/de/trabalho           # onde ficam os arquivos gerados
python3 testdata/generate_scale.py --count 1000 --out $S/lib      # depois: --count 9000 --start 1000 para chegar a 10.000

cat > $S/override.yml <<EOF2
services:
  backend:
    environment:
      CODICE_IMPORT_ROOTS: /import-src
    volumes:
      - $S/lib:/import-src:ro
EOF2
printf 'POSTGRES_PASSWORD=%s\nREDIS_PASSWORD=%s\nJWT_SECRET=%s\nCODICE_PORT=18186\nCODICE_SUBNET=172.30.90.0/24\nCODICE_FRONTEND_IP=172.30.90.10\n' \
  $(openssl rand -hex 12) $(openssl rand -hex 12) $(openssl rand -hex 32) > $S/stack.env
dc() { docker compose -p codice-escala -f docker-compose.full.yml -f $S/override.yml --env-file $S/stack.env "$@"; }
dc up -d --build postgres redis backend worker frontend

# o dono, e o token (arquivo só seu)
curl -s -X POST localhost:18186/auth/setup -H 'Content-Type: application/json' \
  -d '{"username":"dono","password":"uma-senha-de-teste","email":"dono@example.com"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])" > $S/token

# importar (a chamada volta em segundos; o worker leva bem mais) e acompanhar a fila em Administração → Trabalhos
curl -s -X POST localhost:18186/works/bulk-import -H "Authorization: Bearer $(cat $S/token)" -H 'Content-Type: application/json' -d '{"directory":"/import-src"}'

# medir, quando a fila zerar
python3 benchmarks/scale/measure.py --base http://127.0.0.1:18186 --token-file $S/token \
  --manifest $S/lib/manifest.jsonl --label 10k --login-user dono --login-password uma-senha-de-teste
dc down -v
```

O `measure.py` só lê (e entra algumas vezes, abaixo do limite de 10 por minuto). Mede **uma requisição de cada vez** (o que uma pessoa sente) e **20 ao mesmo tempo** (o que uma família sente), e imprime tabelas em Markdown.

## Contra uma instância real (sem o gerador)

O `measure.py` também mede uma biblioteca de verdade: sem `--manifest`, a busca por palavra do título usa uma palavra de um título que a própria biblioteca devolve, a busca pelo marcador sintético é deixada de fora, e **nada do que ele imprime nomeia uma obra** (só tempos, tamanhos e contagens), então a saída pode ser compartilhada. Só lê (e, com `--login-user`, entra algumas vezes, abaixo do limite de 10 por minuto). Rode na máquina do servidor, contra a porta local do site, com uma sessão do dono:

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:8088/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"DONO","password":"..."}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
printf '%s' "$TOKEN" > /tmp/tok && chmod 600 /tmp/tok
python3 benchmarks/scale/measure.py --base http://127.0.0.1:8088 --token-file /tmp/tok --label "alpha, 2.958 obras"
rm -f /tmp/tok   # o arquivo é só seu: apague ao terminar
```

(A porta é a do `CODICE_PORT`; o login só aceita senha local.)

A máquina importa: os números do relatório são de um Mac com 10 CPUs e 8 GB para o Docker; compare sempre **o antes e o depois na mesma máquina**.
