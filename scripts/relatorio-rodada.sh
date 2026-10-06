#!/usr/bin/env bash
# Report of a round of real-world tests (issue #89): the numbers of an instance installed with docker-compose.full.yml, read
# from the server and written as Markdown, so that what goes in the report of the round is measured and not remembered.
#
#   scripts/relatorio-rodada.sh [--com-nomes] [--saida ARQUIVO] [--desde 'AAAA-MM-DD HH:MM']
#
# It only READS: the database session is opened read-only, and nothing is written, restarted or removed. Run it from any folder
# of the machine that runs the stack. By default it says COUNTS and REASONS, never the title or the name of a file, so the report
# can be shared (here, in an issue) without showing what is in the library. With --com-nomes it also lists, for what failed, the
# title of the work and the name of the file: use it for yourself, and read it before sharing.
#
# For a stack with another project name or env file, the usual variables of compose work: COMPOSE_PROJECT_NAME,
# COMPOSE_ENV_FILES.
#
# --desde (UTC) limits the sections of the QUEUE to the jobs created from that moment, so the time of a round is not mixed
# with the jobs of before it (use the time the round began).
set -euo pipefail

names=0
out=""
since=""
while [ $# -gt 0 ]; do
  case "$1" in
    --com-nomes) names=1 ;;
    --saida) shift; out=${1:?--saida needs a file} ;;
    --desde) shift; since=${1:?--desde needs a date and time, like '2026-10-04 21:30'} ;;
    -h|--help) sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (see --help)" >&2; exit 2 ;;
  esac
  shift
done

if [ -n "$since" ] && ! [[ "$since" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}([ T][0-9]{2}:[0-9]{2}(:[0-9]{2})?)?$ ]]; then
  echo "--desde must look like '2026-10-04 21:30' (UTC), got: $since" >&2
  exit 2
fi
jobs_from=""
[ -n "$since" ] && jobs_from="AND created_at >= '$since'::timestamptz"

cd "$(dirname "$0")/.."
compose=(docker compose -f docker-compose.full.yml)

# A session that cannot write: if anything below tried, the server would refuse.
psql_ro() {
  "${compose[@]}" exec -T -e PGOPTIONS='-c default_transaction_read_only=on' postgres \
    sh -c 'psql -X -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -A -F "|" -P footer=off "$@"' sh "$@"
}

# table "title" "SQL": a Markdown table (the first line of the answer is the header). Nothing, when there are no rows.
table() {
  local title=$1 sql=$2 result
  result=$(psql_ro -c "$sql") || { echo "### $title"; echo; echo "_não foi possível ler_"; echo; return 0; }
  echo "### $title"
  echo
  if [ "$(printf '%s\n' "$result" | wc -l | tr -d ' ')" -le 1 ]; then
    echo "_nenhum_"
  else
    printf '%s\n' "$result" | awk -F'|' 'NR==1 { printf "|"; for (i=1;i<=NF;i++) printf " %s |", $i; printf "\n|"; for (i=1;i<=NF;i++) printf "---|"; printf "\n"; next }
      { printf "|"; for (i=1;i<=NF;i++) printf " %s |", $i; printf "\n" }'
  fi
  echo
}
scalar() { psql_ro -t -c "$1" | tr -d ' \n'; }

report() {
  echo "# Relatório da rodada"
  echo
  echo "- **Gerado:** $(date -u +%Y-%m-%dT%H:%M:%SZ) em \`$(hostname)\` ($(uname -sm))"
  echo "- **Versão:** commit \`$(git rev-parse --short HEAD 2>/dev/null || echo desconhecido)\`$(git diff --quiet 2>/dev/null || echo ' (com alterações não commitadas)'), o do repositório de onde a pilha foi construída"
  echo "- **Esquema do banco:** migração $(scalar 'SELECT max(version_id) FROM goose_db_version')"
  echo "- **Nomes de arquivos e obras neste relatório:** $([ "$names" = 1 ] && echo 'SIM (--com-nomes): leia antes de compartilhar' || echo 'não, só contagens e motivos')"
  echo

  echo "## Serviços"
  echo
  echo '```'
  "${compose[@]}" ps --format 'table {{.Service}}\t{{.Status}}' 2>/dev/null || "${compose[@]}" ps
  echo '```'
  echo
  echo "Reinícios desde que os contêineres foram criados (um serviço que reinicia é um serviço que caiu), e se algum foi morto por falta de memória (\`oom\`):"
  echo
  echo '```'
  for id in $("${compose[@]}" ps -q 2>/dev/null); do
    docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}} restarts={{.RestartCount}} oom={{.State.OOMKilled}}' "$id"
  done
  echo '```'
  echo
  echo "Saúde: \`GET /healthz\` responde \`$("${compose[@]}" exec -T backend wget -qO- http://127.0.0.1:8080/healthz 2>/dev/null | tr -d '\r\n' | cut -c1-200)\`"
  echo

  echo "## O acervo"
  echo
  echo "**Obras:** $(scalar 'SELECT count(*) FROM works') ($(scalar 'SELECT count(*) FROM works WHERE retired_at IS NOT NULL') retiradas). **Arquivos:** $(scalar 'SELECT count(*) FROM files')."
  echo
  table "Arquivos por formato e disponibilidade" "SELECT coalesce(format,'(nenhum)') AS formato, availability AS disponibilidade, count(*) AS arquivos, pg_size_pretty(coalesce(sum(size_bytes),0)) AS tamanho FROM files GROUP BY 1,2 ORDER BY 3 DESC"
  table "Obras por estado da análise" "SELECT media_status AS estado, count(*) AS obras FROM works GROUP BY 1 ORDER BY 2 DESC"
  table "Tamanho por modo (gerenciado: dentro do volume da pilha; referenciado: na pasta externa, fora da pilha)" "SELECT sl.mode AS modo, count(*) AS arquivos, pg_size_pretty(coalesce(sum(f.size_bytes),0)) AS tamanho FROM storage_locations sl JOIN files f ON f.id = sl.file_id GROUP BY 1 ORDER BY 2 DESC"
  table "Onde os arquivos ficam" "SELECT mode AS modo, state AS estado, count(*) AS arquivos FROM storage_locations GROUP BY 1,2 ORDER BY 3 DESC"
  table "Idioma declarado pelas edições (os mais comuns)" "SELECT coalesce(language,'(nenhum)') AS idioma, count(*) AS edicoes FROM editions GROUP BY 1 ORDER BY 2 DESC LIMIT 12"
  table "Tamanho dos arquivos (o quanto os reais pesam)" "SELECT CASE WHEN size_bytes IS NULL THEN '(desconhecido)' WHEN size_bytes < 1048576 THEN 'menos de 1 MiB' WHEN size_bytes < 10485760 THEN '1 a 10 MiB' WHEN size_bytes < 104857600 THEN '10 a 100 MiB' WHEN size_bytes < 1073741824 THEN '100 MiB a 1 GiB' ELSE '1 GiB ou mais' END AS tamanho, count(*) AS arquivos FROM files GROUP BY 1 ORDER BY min(coalesce(size_bytes,0))"

  echo "## A fila: o que o worker fez"
  echo
  [ -n "$since" ] && { echo "_Só os trabalhos criados a partir de ${since} (UTC), por causa do --desde._"; echo; }
  table "Trabalhos por tipo e estado" "SELECT type AS tipo, state AS estado, coalesce(error_kind,'') AS tipo_de_erro, count(*) AS trabalhos FROM jobs WHERE true $jobs_from GROUP BY 1,2,3 ORDER BY 1,2"
  table "Quanto cada tipo de trabalho leva (segundos)" "SELECT type AS tipo, count(*) AS trabalhos, round(avg(extract(epoch FROM finished_at-started_at))::numeric,2) AS media_s, round((percentile_cont(0.95) WITHIN GROUP (ORDER BY extract(epoch FROM finished_at-started_at)))::numeric,2) AS p95_s, round(max(extract(epoch FROM finished_at-started_at))::numeric,2) AS maximo_s FROM jobs WHERE state='succeeded' AND started_at IS NOT NULL AND finished_at IS NOT NULL $jobs_from GROUP BY 1 ORDER BY 3 DESC"
  table "Do primeiro trabalho ao último (o tempo de relógio da importação)" "SELECT to_char(min(created_at),'YYYY-MM-DD HH24:MI') AS primeiro, to_char(max(finished_at),'YYYY-MM-DD HH24:MI') AS ultimo_concluido, round((extract(epoch FROM max(finished_at)-min(created_at))/60)::numeric,1) AS minutos, count(*) FILTER (WHERE type='ingest' AND state='succeeded') AS arquivos_lidos FROM jobs WHERE true $jobs_from"
  table "Por que trabalhos falharam (o motivo, com os caminhos cortados, e quantas vezes)" "SELECT type AS tipo, coalesce(error_kind,'') AS tipo_de_erro, left(regexp_replace(coalesce(last_error,'(sem mensagem)'), '/[^:]*', '<caminho>', 'g'), 110) AS motivo, count(*) AS trabalhos FROM jobs WHERE state='failed' $jobs_from GROUP BY 1,2,3 ORDER BY 4 DESC LIMIT 25"
  table "Trabalhos esperando ou rodando agora" "SELECT type AS tipo, state AS estado, count(*) AS trabalhos, to_char(min(created_at),'MM-DD HH24:MI') AS mais_antigo FROM jobs WHERE state IN ('pending','running') GROUP BY 1,2 ORDER BY 3 DESC"

  echo "## Texto, busca e OCR"
  echo
  table "Extração de texto por estado e origem" "SELECT status AS estado, origin AS origem, count(*) AS arquivos, coalesce(sum(segment_count),0) AS trechos, pg_size_pretty(coalesce(sum(char_count),0)) AS caracteres FROM text_extractions GROUP BY 1,2 ORDER BY 3 DESC"
  table "Por que o texto não pôde ser lido (motivos)" "SELECT status AS estado, left(regexp_replace(coalesce(error,'(sem mensagem)'), '/[^:]*', '<caminho>', 'g'), 110) AS motivo, count(*) AS arquivos FROM text_extractions WHERE status IN ('failed','unsupported') GROUP BY 1,2 ORDER BY 3 DESC LIMIT 15"
  table "Idioma detectado no texto" "SELECT coalesce(language,'(nenhum)') AS idioma, count(*) AS arquivos FROM text_extractions WHERE status='ready' GROUP BY 1 ORDER BY 2 DESC LIMIT 12"
  table "OCR das páginas (PDFs escaneados)" "SELECT state AS estado, count(*) AS paginas, count(DISTINCT file_id) AS arquivos FROM ocr_pages GROUP BY 1 ORDER BY 2 DESC"
  table "OCR: por que páginas falharam" "SELECT left(coalesce(error,'(sem mensagem)'), 110) AS motivo, count(*) AS paginas FROM ocr_pages WHERE state='failed' GROUP BY 1 ORDER BY 2 DESC LIMIT 10"

  echo "## Metadados e duplicatas"
  echo
  table "Sugestões de metadados" "SELECT source AS origem, state AS estado, count(*) AS sugestoes FROM metadata_candidates GROUP BY 1,2 ORDER BY 3 DESC"
  table "Possíveis duplicatas" "SELECT reason AS motivo, state AS estado, count(*) AS pares FROM duplicate_candidates GROUP BY 1,2 ORDER BY 3 DESC"
  table "Pessoas: junções propostas" "SELECT state AS estado, count(*) AS propostas FROM person_merge_candidates GROUP BY 1 ORDER BY 2 DESC"

  echo "## Tamanho e peso"
  echo
  echo "**Banco:** $(scalar "SELECT pg_size_pretty(pg_database_size(current_database()))"). **Pasta gerenciada, dentro do contêiner** (os arquivos referenciados ficam fora dela: veja "Tamanho por modo" acima): $("${compose[@]}" exec -T backend sh -c 'du -sh "${CODICE_STORAGE_PATH:-/app/storage}" 2>/dev/null | cut -f1' | tr -d '\r\n')."
  echo
  table "As tabelas mais pesadas" "SELECT relname AS \"tabela\", pg_size_pretty(pg_total_relation_size(oid)) AS tamanho FROM pg_class WHERE relkind='r' AND relnamespace='public'::regnamespace ORDER BY pg_total_relation_size(oid) DESC LIMIT 8"
  echo "Memória e CPU dos contêineres neste momento:"
  echo
  echo '```'
  docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}' $("${compose[@]}" ps -q 2>/dev/null) 2>/dev/null || true
  echo '```'
  echo

  echo "## Pessoas e entradas (só contagens)"
  echo
  table "Contas por papel" "SELECT role AS papel, count(*) AS contas, count(*) FILTER (WHERE blocked_at IS NOT NULL) AS bloqueadas FROM users GROUP BY 1 ORDER BY 2 DESC"
  table "Entradas nos últimos 7 dias" "SELECT method AS caminho, result AS resultado, sum(count) AS tentativas FROM login_events WHERE last_at > now() - interval '7 days' GROUP BY 1,2 ORDER BY 3 DESC"
  table "Sessões abertas por aparelho (o tipo de aparelho, não quem)" "SELECT CASE WHEN user_agent ILIKE '%Android%' THEN 'Android' WHEN user_agent ILIKE '%iPhone%' OR user_agent ILIKE '%iPad%' THEN 'iOS' WHEN user_agent ILIKE '%Windows%' THEN 'Windows' WHEN user_agent ILIKE '%Mac OS%' THEN 'macOS' WHEN user_agent ILIKE '%Linux%' THEN 'Linux' WHEN user_agent IS NULL OR user_agent='' THEN '(nenhum)' ELSE 'outro (aplicativo, OPDS, script)' END AS aparelho, count(*) AS sessoes FROM sessions WHERE revoked_at IS NULL AND expires_at > now() GROUP BY 1 ORDER BY 2 DESC"

  if [ "$names" = 1 ]; then
    echo "## O que falhou, com nomes (só por causa do --com-nomes)"
    echo
    table "Trabalhos que falharam" "SELECT j.type AS tipo, left(w.original_title, 60) AS obra, left(coalesce(j.last_error,''), 160) AS motivo FROM jobs j LEFT JOIN works w ON w.id = j.work_id WHERE j.state='failed' ORDER BY j.finished_at DESC NULLS LAST LIMIT 40"
    table "Arquivos cujo texto não pôde ser lido" "SELECT f.format AS formato, left(w.original_title, 60) AS obra, te.status AS estado, left(coalesce(te.error,''), 160) AS motivo FROM text_extractions te JOIN files f ON f.id = te.file_id JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id WHERE te.status IN ('failed') ORDER BY te.extracted_at DESC LIMIT 40"
    table "Arquivos marcados como ausentes ou bloqueados" "SELECT f.format AS formato, f.availability AS disponibilidade, left(w.original_title, 60) AS obra FROM files f JOIN editions e ON e.id = f.edition_id JOIN works w ON w.id = e.work_id WHERE f.availability <> 'available' LIMIT 40"
  fi

  echo "---"
  echo
  echo "_A preencher à mão no relatório da rodada: a máquina e as pessoas, o que passou, o que falhou (cada falha vira uma issue) e o que não pôde ser testado (isso fica como \"não verificado\")._"
}

if [ -n "$out" ]; then
  umask 077
  report > "$out"
  echo "Relatório gravado em $out" >&2
else
  report
fi
