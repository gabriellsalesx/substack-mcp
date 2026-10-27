# substack-mcp

Servidor MCP para Substack, escrito em Go. É um binário único, sem Python, Node ou navegador embutido, e funciona em qualquer cliente MCP via stdio: Claude Code, Codex, OpenCode, Gemini CLI, Cursor, Claude Desktop e outros.

Com ele você pede ao seu agente para escrever rascunhos em Markdown (com formatação e imagens de verdade), revisar, agendar, publicar, ler posts antigos e postar Notes.

> Projeto não oficial, sem afiliação com a Substack Inc. Ele usa a mesma API interna que o site do Substack usa, que pode mudar sem aviso.

[English version](README.en.md)

## Instalação

Escolha um caminho. Nenhum deles exige Go.

**Com Node.js (npx), sem instalar nada.** Se você já usa Claude Code, Codex ou Gemini CLI, provavelmente já tem Node. O primeiro uso baixa o binário certo para o seu sistema:

```bash
npx -y @gabriellsales/substack-mcp login
```

**Sem Node, com o script de instalação.** Baixa o binário do [GitHub Releases](https://github.com/gabriellsalesx/substack-mcp/releases), confere o checksum e instala em `~/.local/bin` (Linux e macOS) ou em `%LOCALAPPDATA%\Programs\substack-mcp` (Windows):

```bash
curl -fsSL https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/gabriellsalesx/substack-mcp/main/install.ps1 | iex
```

Depois, `substack-mcp login`.

**Outras formas:**

```bash
npm install -g @gabriellsales/substack-mcp                  # ou: pnpm add -g @gabriellsales/substack-mcp
go install github.com/gabriellsalesx/substack-mcp@latest    # com Go 1.26+
```

Ou baixe o arquivo da sua plataforma em [Releases](https://github.com/gabriellsalesx/substack-mcp/releases) e coloque o `substack-mcp` no `PATH`.

## Login

```bash
npx -y @gabriellsales/substack-mcp login
```

Abre o Substack numa janela limpa do seu Chrome, Chromium, Brave ou Edge. Você entra normalmente (código por e-mail ou senha) e, assim que o login termina, a sessão é capturada e a janela fecha sozinha. Nada de DevTools nem de copiar cookie. A janela usa um perfil temporário, apagado ao final; o seu perfil de sempre não é tocado.

Se o navegador não estiver no `PATH`, aponte o caminho com `SUBSTACK_BROWSER=/caminho/do/navegador`. Em servidor sem tela, ou se você usa só Firefox (que não permite esse controle), dá para colar o cookie `substack.sid` à mão:

```bash
substack-mcp login --cookie
pass show substack | substack-mcp login --cookie   # também aceita pipe
```

A sessão fica em `~/.config/substack-mcp/credentials.json` (Linux), com permissão `600`. A senha nunca é pedida nem salva. Outros comandos:

```bash
substack-mcp status   # conta, publicações e publicação padrão
substack-mcp logout   # apaga a sessão local (a do navegador continua)
```

Em CI ou contêiner, dá para pular o login e passar a sessão por variável: `SUBSTACK_SID=s%3A...`.

## Configuração nos clientes

Os exemplos usam `npx`, que funciona sem mexer no `PATH`. Se você instalou pelo script, pelo `npm -g` ou pelo `go install`, troque `npx -y @gabriellsales/substack-mcp` por `substack-mcp` (ou pelo caminho completo que `which substack-mcp` mostrar, se o cliente não achar o comando). A variável `SUBSTACK_PUBLICATION` é opcional: sem ela, vale a publicação escolhida no login.

**Claude Code**

```bash
claude mcp add -s user substack -- npx -y @gabriellsales/substack-mcp
```

**Codex CLI** (`~/.codex/config.toml`)

```toml
[mcp_servers.substack]
command = "npx"
args = ["-y", "@gabriellsales/substack-mcp"]
# Pede aprovação para tudo que não é somente leitura.
default_tools_approval_mode = "writes"
```

**OpenCode** (`opencode.json`)

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "substack": { "type": "local", "command": ["npx", "-y", "@gabriellsales/substack-mcp"], "enabled": true }
  }
}
```

**Gemini CLI**

```bash
gemini mcp add -s user substack npx -y @gabriellsales/substack-mcp
```

**Cursor** (`~/.cursor/mcp.json`) e **Claude Desktop** (`claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "substack": {
      "command": "npx",
      "args": ["-y", "@gabriellsales/substack-mcp"]
    }
  }
}
```

No Windows, se o cliente não encontrar o `npx`, use `"command": "cmd"` e `"args": ["/c", "npx", "-y", "@gabriellsales/substack-mcp"]`.

Reinicie o cliente depois de configurar e peça algo como: *"liste meus últimos rascunhos no Substack"*.

### Comandos

| Comando | O que faz |
| --- | --- |
| `substack-mcp` | Roda o servidor MCP via stdio (é o que o cliente chama). |
| `substack-mcp login` | Conecta sua conta pelo navegador. `--cookie` cola o cookie à mão; `--publication` define a publicação padrão. |
| `substack-mcp status` | Mostra a conta, as publicações e a padrão. |
| `substack-mcp logout` | Apaga a sessão salva neste computador. |
| `substack-mcp version` | Mostra a versão. |

Com `npx`, prefixe com `npx -y @gabriellsales/substack-mcp`, por exemplo `npx -y @gabriellsales/substack-mcp status`.

## Ferramentas

| Ferramenta | O que faz |
| --- | --- |
| `list_posts` | Lista rascunhos, agendados ou publicados, com ids e links. |
| `get_post` | Lê um post inteiro como Markdown. |
| `create_draft` | Cria rascunho a partir de Markdown e sobe as imagens locais. |
| `update_draft` | Altera título, subtítulo, corpo, público ou seção. |
| `duplicate_post` | Copia um post, com formatação, para um novo rascunho. |
| `publish_post` | Publica agora ou agenda, com a checagem do editor na prévia. Exige confirmação. |
| `delete_draft` | Apaga um rascunho ou cancela um agendado. Exige confirmação. |
| `upload_image` | Sobe uma imagem para o CDN do Substack. |
| `get_publication` | Publicação, assinantes, visualizações, seções e outras publicações da conta. |
| `create_note` | Posta ou agenda uma Note. Exige confirmação. |
| `list_scheduled_notes` | Lista Notes agendadas. |
| `delete_note` | Cancela ou apaga uma Note. Exige confirmação. |
| `create_community_post` | Abre conversa no chat de assinantes. Exige confirmação. |

Toda ferramenta aceita `publication` opcional (URL, domínio ou subdomínio) para quem tem mais de uma publicação.

### Confirmação antes de publicar

As ferramentas que chegam até leitores ou apagam algo não agem na primeira chamada. Sem `confirm: true`, elas devolvem uma prévia do que fariam (título, público, quantos assinantes recebem o e-mail) e nada muda. O agente mostra a prévia, você aprova e ele chama de novo com `confirm: true`. Além disso, as ferramentas trazem as anotações MCP (`readOnlyHint`, `destructiveHint`), que clientes como Codex e Claude Code usam para pedir permissão.

`send_email` é `false` por padrão. Publicar não dispara e-mail para a base a menos que você peça.

### Markdown suportado

Títulos, negrito, itálico, tachado, código inline, links, listas (inclusive aninhadas), citações, blocos de código com linguagem, linha horizontal e imagens com legenda (`![alt](foto.png "legenda")`). Imagens locais ou externas são enviadas para o CDN do Substack automaticamente. Uma linha `<!-- paywall -->` marca onde começa o conteúdo pago, e `<!-- subscribe -->` insere o botão de inscrição com o mesmo texto dos seus posts anteriores. Tabelas viram bloco de código, porque o Substack não tem tabela.

Um `# Título` no início do texto, igual ao título do post, é removido para não aparecer duas vezes.

## Variáveis de ambiente

| Variável | Uso |
| --- | --- |
| `SUBSTACK_PUBLICATION` | Publicação padrão (URL, domínio ou subdomínio). `SUBSTACK_PUBLICATION_URL` também vale. |
| `SUBSTACK_SID` | Valor do cookie de sessão; tem prioridade sobre o login salvo. |
| `SUBSTACK_COOKIE` | Cabeçalho `Cookie` completo; tem prioridade sobre `SUBSTACK_SID`. |
| `SUBSTACK_USER_AGENT` | User-Agent das requisições. |
| `SUBSTACK_BROWSER` | Caminho do navegador usado no `login`. |
| `SUBSTACK_MCP_CONFIG_DIR` | Pasta do `credentials.json`. |
| `SUBSTACK_DEBUG=1` | Loga as chamadas HTTP no stderr (sem cookies). |

## Limitações conhecidas

- A API é não oficial. Se o Substack mudar um endpoint, a ferramenta correspondente pode falhar até uma nova versão.
- O login automático precisa de um navegador baseado em Chromium; sem ele, use `--cookie`.
- Publicações com domínio próprio são acessadas pelo endereço `*.substack.com`, que é onde vale o cookie de sessão.
- Editar um post já publicado altera a versão pendente; para ir ao ar, chame `publish_post` com `send_email: false`.
- A contagem de assinantes não aparece para todas as contas.

## Desenvolvimento

```bash
go test ./...
go build -o substack-mcp .
```

O pacote `internal/prosemirror` converte Markdown para o formato de documento do Substack e de volta; `internal/substack` é o cliente HTTP; `internal/browser` faz o login pelo navegador; `internal/server` registra as ferramentas MCP com o [SDK oficial em Go](https://github.com/modelcontextprotocol/go-sdk).

### Publicar uma versão

```bash
git tag v0.2.0 && git push origin v0.2.0
```

O workflow `release` compila os binários com o GoReleaser, cria o Release no GitHub com `checksums.txt` e publica no npm o pacote principal mais um pacote por plataforma (`npm/build.mjs` e `npm/publish.mjs`). O CI roda testes em Linux, macOS e Windows, `golangci-lint` e `govulncheck`. Para testar localmente: `node npm/build.mjs 0.2.0 && node npm/publish.mjs --dry-run`.

## Autor

Criado e mantido por **Gabriell Sales**.

- GitHub: [@gabriellsalesx](https://github.com/gabriellsalesx)
- Substack: [GS Lab](https://gabriellsales.substack.com)

Contribuições são bem-vindas: abra uma issue ou um pull request.

## Licença

MIT. Veja [LICENSE](LICENSE).
