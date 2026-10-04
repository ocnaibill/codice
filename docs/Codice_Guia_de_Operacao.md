# Códice: guia de operação

Para quem **mantém uma instância no ar** (a sua, de uma família, de um grupo pequeno). Não é a especificação: é o que fazer no primeiro dia, o que olhar na rotina, como atualizar e como se recuperar. Os detalhes de instalação e de backup estão no [README](../README.md); aqui eles aparecem como roteiro, com o que é decisão sua marcada como tal.

Todos os comandos supõem a pasta do repositório e a pilha de `docker-compose.full.yml`. Para não repetir, o guia usa:

```bash
alias dc='docker compose -f docker-compose.full.yml'
```

## 1. Primeiro dia

1. **Segredos.** `cp .env.example .env` e troque **todos** os valores de exemplo (`openssl rand -hex 24`; caracteres seguros para URL). Obrigatórios: `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `JWT_SECRET`. A API se recusa a subir com o `JWT_SECRET` de exemplo. **Guarde uma cópia do `.env` fora do servidor:** ele não vai em nenhum backup.
2. **Subir.** `dc up -d --build`. Abra `http://localhost:8080`; na primeira visita você cria o **dono**.
3. **Alcance.** O site só escuta em `127.0.0.1`. Para outros aparelhos da rede, `CODICE_BIND=0.0.0.0`. Para a internet, deixe em `127.0.0.1` e ponha na frente um proxy ou túnel com HTTPS (o Códice não termina TLS). Atrás de um proxy ou túnel defina `CODICE_PUBLIC_URL=https://seu.endereco` (senão os links do OPDS saem em `http://`) e, se o proxy roda **no próprio host**, `CODICE_TRUSTED_PROXIES` (ver a seção 6).
4. **Backup e ensaio, antes de pôr gente.** Configure o backup (seção 4) e **restaure uma vez num projeto separado** para ver funcionar e medir o tempo. Um backup que nunca foi restaurado não é garantia.
5. **Conferir a saúde.** `dc ps` deve mostrar tudo `healthy`; em *Administração → Sistema* a versão, o espaço livre e o estado do backup.
6. **Convidar.** *Administração → Contas*: convites e papéis. Quem entra por diretório (Authentik/LDAP) está em *Login externo* (só o dono) e no README.

## 2. Rotina: o que olhar

| Onde | O que mostra | Quando olhar |
|---|---|---|
| `dc ps` | estado e saúde de cada serviço | depois de qualquer mudança, e se algo parecer lento |
| *Administração → Sistema* | versão, espaço livre, último backup (idade, se foi verificado, se está no mesmo disco do acervo) | uma vez por semana; o aviso fica vermelho depois de um dia e meio sem backup |
| *Administração → Trabalhos* | a fila: o que está rodando, o que falhou e por quê | depois de enviar muita coisa, ou se um livro ficar "Na fila" |
| *Administração → Entradas* | quem tentou entrar, de onde, com que resultado | se suspeitar de tentativas de invasão; o prazo de guarda (90 dias por padrão, de 7 a 3650) é ajustável pelo dono |
| *Administração → Lixeira* e *Armazenamento* | o que está para ser apagado e o que sobrou órfão | de vez em quando, antes de o disco apertar |
| `GET /healthz` | `ok`, `degraded` (Redis fora, nada se perde) ou `down` (sem banco) | para um monitor externo (Uptime Kuma, etc.) |

**Um livro que fica "Na fila" para sempre** quase sempre é o **worker parado**: `dc ps worker`, `dc logs --tail 100 worker`. O envio e a leitura continuam funcionando sem o worker; só a extração de metadados e capas espera.

## 3. Atualizar e voltar atrás

```bash
scripts/backup.sh /mnt/backups/codice        # SEMPRE antes (com CODICE_BACKUP_PASSPHRASE definida)
git pull
CODICE_VERSION=$(git rev-parse --short HEAD) dc up -d --build
dc ps                                         # tudo healthy?
```

- As **migrações do banco rodam sozinhas** quando a API sobe. Elas são feitas para ir **para frente**: a imagem de produção não traz um comando para desfazê-las.
- **Voltar atrás** é, portanto, **restaurar o backup feito antes de atualizar** com o código da versão anterior (`git checkout` da versão antiga, `dc up -d --build`, restaurar; ver a seção 4). O `restore` recusa um pacote de um Códice **mais novo** que o código em uso: é por isso que a ordem importa.
- Anote a versão em uso antes (a tela "Sobre", ou `git rev-parse --short HEAD`); ela é o que você precisa para voltar.
- Atualizar em horário calmo: durante o `up -d --build` o site cai por alguns segundos.

## 4. Backup e restauração

O essencial (o detalhe, as opções e as medidas de tempo estão no README, seção "Backup e restauração"):

- **O Códice não agenda backup; você agenda.** Um por dia, no `cron`: `0 3 * * * CODICE_BACKUP_PASSPHRASE="$(cat /etc/codice-frase)" /caminho/do/codice/scripts/backup.sh /mnt/backups/codice`. O script grava com permissão `600`, aplica a retenção (7 diários, 4 semanais, 3 mensais) e não deixa arquivo parcial.
- **A frase-senha** criptografa o pacote. **Sem ela o pacote não abre**: guarde-a fora do servidor, junto do `.env`.
- **Decisão sua:** o destino. Um backup **no mesmo disco** do acervo protege de engano, **não** de disco que morre; o painel avisa quando está no mesmo disco. O ideal é outro disco ou outra máquina.
- **Pelo painel:** o dono pode *fazer um backup agora* e *verificar* um pacote (precisa da pasta e do arquivo com a frase configurados; ver `.env.example`). **Restaurar e baixar o pacote ficam fora do navegador, de propósito.**
- **Verificar** de vez em quando: `verify-backup --deep` ensaia a restauração num banco temporário e diz quanto tempo levou.
- **Restaurar:** `dc up -d postgres redis`, depois `codice-admin restore --in - < pacote`, depois `dc up -d`. Numa instância que já tem dados, só com `--overwrite`, e mesmo assim o banco atual é **renomeado e guardado**, não apagado. Pare a API e o worker antes.
- **O que o pacote não tem e é seu guardar:** o `.env`, a pasta `ldap/` (segredos e certificado), as pastas da biblioteca externa (modo referenciado). Sessões, tokens de aplicativo, convites e o registro de entradas **nunca** entram; depois de restaurar, todos precisam entrar de novo.

## 5. Contas e acesso

- **O dono perdeu a senha ou o segundo fator:** só quem controla o servidor resolve.
  `dc exec backend codice-admin recover-owner --base-url https://seu.endereco` encerra as sessões do dono e imprime um link de redefinição válido por **uma hora**.
- **O dono está indisponível:** `codice-admin transfer-owner --to USUARIO` (o antigo dono vira leitor, a não ser que `--former-role` diga outra coisa). Os dois comandos ficam no **log de auditoria** e **aparecem para as pessoas afetadas** no próximo login.
- **Um aparelho perdido ou uma sessão suspeita:** a própria pessoa encerra em *Minha conta → Sessões e dispositivos*; o dono ou um administrador pode encerrar as de outra conta em *Administração → Contas* (encerrar não bloqueia: a pessoa entra de novo com a senha). Para cortar o acesso de vez, use **Bloquear** (*Administração → Contas*): encerra na hora as sessões, os tokens de aplicativo e as conexões abertas, guarda as notas e o progresso, e se desfaz com *Desbloquear*.
- **Auditoria:** as ações administrativas ficam na tabela `audit_log`, ainda **sem tela própria**. Para lê-la:
  ```bash
  dc exec postgres psql -U codice_user -d codice_db \
    -c "select at, actor_username, action, target_type, target_id from audit_log order by at desc limit 50"
  ```
  (troque o usuário e o banco se você mudou `POSTGRES_USER` e `POSTGRES_DB`.) A tabela só aceita inserir, não alterar nem apagar.
- **Exportar meus dados:** cada pessoa gera o próprio ZIP em *Minha conta → Preferências → Meus dados*; os arquivos ficam no máximo 24 horas (`CODICE_EXPORT_DIR`).

## 6. Rede, HTTPS e quem é o cliente

- **Só o site tem porta publicada.** A API, o banco e o Redis ficam na rede interna da pilha.
- **O login tem limite por cliente.** A API só acredita no `X-Forwarded-For` do contêiner do site (endereço fixo `CODICE_FRONTEND_IP`). Se o seu proxy ou túnel roda **no mesmo host**, as conexões dele chegam do gateway da rede (o `.1` de `CODICE_SUBNET`) e, sem aviso, **todos os usuários dividiriam um limite só**: defina `CODICE_TRUSTED_PROXIES=172.30.77.10,172.30.77.1` (ajuste ao seu `CODICE_SUBNET`), sabendo que isso significa "acredite no que o proxy do host disser". Confira em *Administração → Entradas* se o endereço mostrado é o das pessoas e não o do proxy.
- **Sub-rede em conflito:** mude `CODICE_SUBNET` e `CODICE_FRONTEND_IP` (um endereço dentro dela, que não seja o `.1`).
- **A página sai com política de conteúdo (CSP)** e cabeçalhos de segurança do próprio nginx do site; **um proxy na frente não deve removê-los nem sobrescrevê-los**.
- **Endereço público:** `CODICE_PUBLIC_URL` vale para **todos** os links do OPDS. Se o domínio atende também em `http://`, ative o redirecionamento para HTTPS no proxy.

## 7. Disco e crescimento

| O que | Onde | Como cresce | O que fazer |
|---|---|---|---|
| A biblioteca e as capas | volume `codice_storage` | com cada envio | olhe o espaço livre em *Sistema*; a *Lixeira* guarda antes de apagar |
| O banco | volume `codice_pgdata` | notas, destaques e, principalmente, **o texto para a busca** | o índice de busca pode pesar **várias vezes o texto** dos livros (6,5 vezes na medição com texto sintético): reserve espaço; o backup não o leva e ele é refeito depois de restaurar |
| Pacotes de backup | volume `codice_backups` ou `CODICE_BACKUP_HOST_DIR` | um por dia, até a retenção | de preferência em **outro disco** |
| Modelos locais (opcional) | volume `codice_model_cache` | só se o dono ligar a IA local | baixa só o modelo escolhido |
| Registro de entradas | tabela `login_events` | uma linha por tentativa | teto de linhas e prazo ajustável pelo dono (7 a 3650 dias) |

Os serviços opcionais (`embeddings` e `ocr`) **sobem parados** e só trabalham quando o dono os liga no app; se a máquina é pequena, dá para não os subir (`dc up -d postgres redis backend worker frontend`).

## 8. Logs

```bash
dc logs --tail 200 backend          # a API (inclui migrações e o que ela entende de proxy/LDAP ao subir)
dc logs -f worker                   # o worker, em tempo real
dc logs --since 1h frontend         # os acessos (nginx)
```

Ao subir, a API registra se o LDAP está ligado, em quantos proxies confia e o endereço público dos links: é o primeiro lugar para conferir uma configuração.

## 9. Os números da sua instância

`scripts/relatorio-rodada.sh` lê o servidor e escreve um relatório em Markdown: serviços (com reinícios e falta de memória), o acervo por formato e tamanho, a fila (tempo por tipo de trabalho e **por que os trabalhos falharam**), texto e OCR, tamanho do banco e da biblioteca, memória e CPU dos contêineres, contas e entradas. **Só lê**: a sessão do banco é aberta como somente leitura. Por padrão traz só contagens e motivos, sem o título de nenhuma obra nem o nome de arquivo, então dá para compartilhar; com `--com-nomes` lista também o que falhou, com nomes, para uso seu. É o que alimenta o relatório de uma rodada de testes reais (modelo em [`Codice_Rodada_de_Testes_MODELO.md`](Codice_Rodada_de_Testes_MODELO.md)), e serve também para olhar a saúde da instância de vez em quando.

## 10. Quando algo dá errado

| Sintoma | Causa provável | O que fazer |
|---|---|---|
| A API não sobe | `JWT_SECRET` de exemplo, ausente, ou `CODICE_PUBLIC_URL`/LDAP inválidos | `dc logs backend`: a mensagem diz qual variável |
| Todo mundo trava no limite de login | proxy do host não está em `CODICE_TRUSTED_PROXIES` | seção 6 |
| Links do OPDS em `http://` atrás de HTTPS | falta `CODICE_PUBLIC_URL` | definir; reiniciar a API |
| Livros ficam "Na fila" | worker parado ou sem alcançar o banco | `dc ps worker`; `dc logs worker`; ele volta sozinho quando o banco volta |
| Os botões de backup do painel estão desligados | falta a pasta de backups **ou** o arquivo da frase | o painel diz qual; ver `.env.example` |
| `restore` recusa | há conexões abertas, o banco já tem dados, ou o pacote é de um Códice mais novo | parar `backend worker frontend`; `--overwrite` só se for isso mesmo; usar o código da versão do pacote |
| `restore` ou `verify-backup` não abre o pacote | frase errada, ou pacote corrompido | a frase é a do **momento do backup**; tentar o pacote anterior |
| Contêiner `unhealthy` | o worker não alcança o banco, ou a API não responde em `/healthz` | `dc logs`; o worker volta a `healthy` sozinho |
| Não consigo entrar como dono | senha ou segundo fator perdidos | seção 5 (`recover-owner`) |
| Conflito de rede ao subir | a sub-rede padrão já existe | seção 6 |

## 11. O que este guia ainda não cobre

Dito com franqueza, para ninguém achar que existe:

- **Não há tela de auditoria** (só a consulta no banco, acima), nem agendamento de backup pela interface.
- **Não há monitoramento embutido**: use `GET /healthz` com um monitor seu.
- **O tempo de restauração com o acervo real do mantenedor ainda não foi medido** (as medidas do README são sintéticas); faça o ensaio da seção 1 com o **seu** acervo.
- **Não há guia de capacidade fechado** (quantas obras e quantas pessoas cabem em que máquina). Há uma primeira medição com 10.000 obras em [`Codice_Teste_de_Escala_2026-10-04.md`](Codice_Teste_de_Escala_2026-10-04.md): a lista, a busca e o OPDS respondem em menos de 0,6 s, e trazer o acervo leva ~3 obras por segundo; **o índice de busca ocupa muito mais disco que o texto** (6,5 vezes, com texto sintético: o número real depende dos seus livros), então **reserve espaço no banco** (`codice_pgdata`) para isso.
- **Atualizar sem parada** e **alta disponibilidade** não existem: é uma instância, em uma máquina.
