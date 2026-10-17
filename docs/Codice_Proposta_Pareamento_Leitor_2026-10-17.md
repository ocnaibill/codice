# Proposta: pareamento do leitor por código (quick connect) e configuração versionada

**Decidida em 17/10/2026 (DEC-194); backend implementado.** Parte da issue [#327](https://github.com/ocnaibill/codice/issues/327) (leitor do Códice para e-readers, plugin sobre o KOReader). O mantenedor decidiu em 16/10/2026 que o pareamento vem antes de qualquer tela nova e que ele deve também fazer o leitor **puxar da instância a configuração** (OPDS, KOSync e o que vier), com número de versão. Esta proposta desenha o endpoint do Códice e o fluxo do plugin; as decisões que cabem ao mantenedor estão na seção 8. Nada aqui está implementado.

## 1. O que o protótipo já mostrou (17/10/2026, Kindle 10ª geração, KOReader v2026.07.2)

Um plugin Lua fala HTTPS com a instância, aplica uma entrada nova no OPDS sem reiniciar, desenha capas numa tela própria e recebe os eventos de anotação. O KOSync lê `custom_server`, `username` e `userkey` a cada chamada, então mudar a configuração dele vale na hora, dentro de um livro. O widget de QR (`ui/widget/qrmessage`) existe na versão usada.

## 2. O problema

Hoje a pessoa baixa `opds.lua` e `kosync.lua` em *Minha conta → Aplicativos*, copia por USB para o Kindle e, se algo muda no Códice, repete. A senha curta fica em texto simples nesses arquivos. O pareamento troca isso por: o leitor mostra um código, a pessoa o aprova no celular ou no computador onde já está logada, e o leitor recebe sozinho o acesso e a configuração.

## 3. O fluxo (autorização de dispositivo, no estilo da RFC 8628)

```
 Leitor (plugin)                    Servidor                       Pessoa (web, logada)
 POST /device/code  ───────────────▶ cria o pedido
   ◀─ deviceCode, userCode, URL, QR, validade, intervalo
 mostra o código e o QR
                                                                   abre /pair?code=…  (ou lê o QR)
                                                                   vê aparelho, hora, IP; Aprova
 POST /device/token (a cada N s) ──▶ pendente… … aprovado:
   ◀─ cria o acesso (escopo koreader) e o entrega UMA vez
 GET /device/config  ──────────────▶ configuração versionada
 aplica: OPDS, KOSync (e o que vier)
```

### 3.1 Endpoints do leitor (sem sessão; limitados por IP)

| Endpoint | Corpo | Resposta |
|---|---|---|
| `GET /device/info` | nenhum | `{name, version, pairing: true}`: o plugin confere que o endereço digitado é um Códice com pareamento ligado |
| `POST /device/code` | `{client: "codice-ereader/0.1", platform: "kindle", name: "Kindle de Ana"}` (nome opcional, até 60 caracteres) | `{deviceCode, userCode: "BCDF-2345", verificationUri, verificationUriComplete, expiresIn: 600, interval: 5}` |
| `POST /device/token` | `{deviceCode}` | pendente: `400 {error: "authorization_pending"}`; rápido demais: `400 {error: "slow_down"}`; vencido: `400 {error: "expired_token"}`; negado: `400 {error: "access_denied"}`; aprovado: `200 {token, username, config}` (uma vez só) |

### 3.2 Endpoints da pessoa (com sessão)

| Endpoint | Efeito |
|---|---|
| `GET /auth/device/{userCode}` | o que a página `/pair` mostra: nome e plataforma informados pelo aparelho, hora do pedido, IP de quem pediu |
| `POST /auth/device/{userCode}/approve` | marca o pedido como aprovado por esta conta |
| `POST /auth/device/{userCode}/deny` | nega |

### 3.3 Configuração versionada (com o acesso do leitor)

`GET /device/config` com o mesmo `Basic` do OPDS (usuário e acesso curto). Resposta e cabeçalho `ETag`/`If-None-Match`:

```json
{
  "version": 1,
  "instance": {"name": "Códice", "url": "https://codice.exemplo.com"},
  "opds":   {"title": "Códice", "url": "https://codice.exemplo.com/opds/v1.2/catalog"},
  "kosync": {"url": "https://codice.exemplo.com/kosync"},
  "features": {"sendToKindle": true, "notesImport": true},
  "minPlugin": "0.1.0"
}
```

O plugin guarda a última `version`, pergunta ao abrir e quando a rede volta, e aplica **só o que mudou**, de forma idempotente (a entrada do OPDS é identificada pelo título "Códice"; o KOSync recebe `custom_server`, `username` e `userkey = md5(acesso)`, o mesmo que o servidor já aceita). Um endpoint novo do Códice chega ao aparelho sem reinstalar nada; `minPlugin` permite pedir atualização do plugin quando a configuração usa algo que ele não conhece.

## 4. Segurança

- **Código do usuário** (`userCode`): 8 símbolos de um alfabeto sem ambiguidade (o mesmo da senha curta, 31 símbolos, ~39 bits), maiúsculas e minúsculas valem igual, o hífen é só para ler. Um código errado não pertence a pedido nenhum, então o que freia o chute é o **limite de taxa** por endereço em `/auth/device/*` e em `/device/*`, somado à validade curta e ao limite de pedidos esperando.
- **Código do aparelho** (`deviceCode`): 128 bits aleatórios, guardado só como hash (sha256). Nunca aparece na tela.
- **Validade** de 10 minutos; no máximo 5 pedidos pendentes por IP; o servidor impõe o intervalo de consulta (`slow_down`).
- **O acesso nasce na primeira consulta depois da aprovação**, e não na aprovação: o texto do acesso nunca é guardado em lugar nenhum, só entregue. Consulta repetida depois disso devolve `expired_token`.
- **Phishing por código** (alguém inicia o fluxo e convence a vítima a aprovar): a página `/pair` mostra aparelho, hora e IP de quem pediu e diz "aprove só se o código está agora na tela do seu leitor"; o QR leva ao mesmo endereço, com o código já preenchido, para comparar. O acesso criado aparece em *Aplicativos* com o nome e o último uso, e se revoga ali.
- **Auditoria** (`device.pair.request`, `device.pair.approve`, `device.pair.deny`, `device.pair.token`): quem, aparelho, IP; **nunca** o código do aparelho nem o acesso.
- O pareamento pode ser desligado pelo dono (configuração da instância); desligado, `GET /device/info` diz `pairing: false`.

## 5. Dados

Tabela `device_pairings` (migração própria):

```sql
id BIGSERIAL PRIMARY KEY,
device_code_hash BYTEA UNIQUE NOT NULL,
user_code TEXT UNIQUE NOT NULL,             -- sem hífen, em maiúsculas
name TEXT, platform TEXT, client TEXT,
request_ip INET,
created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
expires_at TIMESTAMPTZ NOT NULL,
last_poll_at TIMESTAMPTZ,
approved_by UUID REFERENCES users(id) ON DELETE CASCADE,
approved_at TIMESTAMPTZ, denied_at TIMESTAMPTZ, consumed_at TIMESTAMPTZ
```

Pedidos vencidos ou consumidos são apagados por uma limpeza periódica (a mesma que já cuida dos outros temporários).

## 6. O que muda em cada lugar

- **Backend:** handlers `device.go` (os seis endpoints), a configuração (`device_config.go`, com a `version` como constante de código somada a uma mudança de endereço público), migração, auditoria, limite de taxa. O acesso criado usa `CreateScopedAppToken` com escopo `koreader` (a senha curta), como hoje.
- **Frontend:** a página `/pair` (código, dados do pedido, Aprovar e Negar) e, em *Aplicativos*, o botão **Conectar um leitor** que leva a ela. Os arquivos `opds.lua` e `kosync.lua` continuam disponíveis como plano B.
- **Plugin (`codice-ereader-plugin`):** tela "Conectar ao Códice" (endereço, código, QR com contagem regressiva, consulta a cada `interval` segundos com `UIManager:scheduleIn`, mantendo a rede ligada), `settings/codice.lua` com o endereço, o usuário e a versão da configuração, e o aplicador da configuração.

## 7. Plano de implementação

1. Backend + testes de integração (fluxo feliz, vencimento, negação, tentativas erradas, `slow_down`, entrega única do acesso, aprovação por outra conta, auditoria sem o acesso).
2. Página `/pair` + botão em *Aplicativos* + testes.
3. Plugin: pareamento (QR e código) e aplicação da configuração; teste no Kindle.
4. Guia de operação (seção 6b) e matriz QA-018.

## 8. Decisões que cabem ao mantenedor

1. **Aprovar exige a senha de novo?** Recomendação: não; a sessão já basta, como no Jellyfin. A página mostra o aparelho e o IP, e tudo se revoga.
2. **Nome do aparelho:** o plugin sugere "Kindle" + modelo e a pessoa pode renomear em *Aplicativos*? Recomendação: sim.
3. **Pareamento ligado por padrão?** Recomendação: ligado, com chave para o dono desligar.
4. **Vários leitores por conta:** sem limite além do que as contas já têm de acessos? Recomendação: sim.
5. **O QR é obrigatório na primeira versão?** Recomendação: não; o código digitado resolve, e o QR entra se o widget se comportar bem no aparelho (ele existe na versão testada).
