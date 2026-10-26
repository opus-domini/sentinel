---
name: execute-plan
description: Executa integralmente um implementation-plan.md existente, marco a marco, com goal persistente quando autorizado, validação proporcional, atualização fiel dos checks e um commit local por marco. Use quando o usuário pedir para implementar, executar ou concluir um plano técnico já escrito.
metadata:
  author: opus-domini
  version: "1.0.0"
---

# Execute Plan

Execute um plano de implementação existente como checklist vivo e fonte de
verdade. Entregue o estado-alvo completo; não trate o plano como sugestão e não
declare tarefas concluídas sem evidência.

## Contrato de execução

Ao usar esta skill:

- leia integralmente o `implementation-plan.md` indicado antes de alterar código;
- leia os artefatos de entrada diretamente referenciados pelo plano quando forem
  necessários para entender decisões, limites ou critérios de aceite;
- respeite `AGENTS.md`, instruções do repositório e decisões fechadas do plano;
- audite código, testes e callers atuais antes de cada mudança relevante;
- execute um marco por vez, na ordem e com as dependências descritas;
- mantenha o repositório compilável e testável ao final de cada marco;
- marque somente checks efetivamente comprovados;
- rode as validações previstas, incluindo gates do projeto;
- atualize documentação quando o plano exigir;
- crie o commit local de cada marco somente depois de seu fechamento completo;
- não inicie o marco seguinte antes de confirmar o commit do atual;
- faça hard cut quando esse for o contrato do plano;
- não adicione compatibility layers, fallbacks, flags ou implementações paralelas
  para contornar uma decisão já fechada;
- não use subagentes ou delegação salvo pedido explícito do usuário;
- continue até concluir todo o plano ou encontrar um bloqueio real que não possa
  ser resolvido com inspeção e ações seguras dentro do escopo.

## Autorizações e limites

O pedido explícito para executar um plano autoriza, dentro do repositório e do
escopo descrito:

- editar, criar, mover e remover arquivos necessários à implementação;
- remover código antigo substituído pelo novo contrato;
- executar builds, formatters, linters e testes;
- atualizar fixtures, snapshots e documentação afetada;
- instalar ou reiniciar o serviço local quando isso fizer parte da validação;
- inspecionar banco, config, logs e runtime local de forma não destrutiva;
- produzir dados de teste controlados e reversíveis;
- usar browser automation para validar a aplicação local quando houver impacto
  de UI;
- criar um commit local Conventional Commit ao final de cada marco integralmente
  validado, sem pedir nova confirmação.

A invocação explícita de `$execute-plan` torna esses commits locais obrigatórios e
recomendados: exatamente um commit coerente por marco, depois dos checks, gates e
revisão de diff previstos. Se o plano não fornecer mensagem, derive uma mensagem
Conventional Commits fiel ao marco. Nunca inclua `.notes`, `.refs`, config local,
segredos ou evidência efêmera no commit.

O pedido não autoriza automaticamente:

- push, branch, PR, merge, release ou deploy;
- escrita em serviços externos, mensagens a terceiros ou mudança de recursos
  remotos;
- apagar ou sobrescrever dados do usuário;
- reset destrutivo de worktree, banco, config ou runtime;
- ampliar o escopo para refactors não necessários ao estado-alvo.

Essas ações só podem ocorrer quando o usuário ou o próprio plano, autorizado pelo
usuário, as pedir de forma inequívoca. Push continua proibido quando apenas commit
local foi autorizado. Antes de qualquer ação destrutiva autorizada, resolva o alvo
exato, faça preflight somente leitura e prefira backup ou operação recuperável.

Mudanças locais preexistentes pertencem ao usuário. Nunca use reset, checkout,
clean, stash ou sobrescrita para escondê-las. Trabalhe ao redor delas; pare e peça
direção apenas quando houver sobreposição material impossível de resolver com
segurança.

## Uso de goal persistente

Use as ferramentas de goal sempre que esta skill for invocada. A invocação
explícita de `$execute-plan` autoriza criar ou retomar o goal correspondente, sem
o usuário precisar repetir `set_goal` ou “defina como meta”. Se o usuário pedir
execução de plano sem invocar esta skill, use goal somente quando ele solicitar
explicitamente.

Quando autorizado:

1. consulte o goal atual;
2. se não houver goal ou o anterior estiver completo, crie um goal que cite o
   caminho exato do plano, os marcos, a política de checks, commits e push;
3. se já existir um goal ativo para o mesmo plano, continue-o em vez de criar
   outro;
4. se houver outro goal ativo e inacabado, não o substitua silenciosamente;
   reporte o conflito e obtenha direção;
5. não defina `token_budget` sem orçamento explícito do usuário;
6. mantenha o goal ativo enquanto houver tarefa, validação, check ou commit
   obrigatório pendente;
7. marque `complete` somente quando todo o objetivo estiver realmente concluído;
8. marque `blocked` somente segundo a regra da ferramenta: a mesma condição deve
   impedir progresso por pelo menos três turnos consecutivos;
9. dificuldade, teste demorado ou necessidade de correção não são bloqueio por si
   só.

O goal deve sobreviver a retomadas e compactações. Ao retomar, releia o plano,
confirme os checks já marcados contra commits/evidências e continue do primeiro
item aberto; não repita trabalho já fechado.

## Preflight obrigatório

Antes da primeira edição produtiva:

1. resolva o caminho canônico do plano e leia o arquivo até EOF;
2. identifique todos os marcos, dependências, commits, validações e pontos em
   aberto;
3. confirme que o plano pertence ao repositório atual;
4. leia `AGENTS.md` aplicável e os materiais diretamente governantes;
5. inspecione branch, `git status --short`, commit atual e histórico recente;
6. confirme se o plano está ignorado ou versionado;
7. inventarie mudanças locais preexistentes e preserve-as;
8. confira comandos reais no Makefile, scripts, package manifests e docs;
9. trace os principais tipos, stores, APIs, callers, telas e testes citados;
10. transforme qualquer drift descoberto em ajuste explícito do plano antes de
    implementar, sem renumerar IDs já existentes;
11. verifique se existe ponto em aberto que realmente impeça o primeiro marco.

Por padrão, trabalhe diretamente na branch atual esperada pelo plano. Não crie nem
troque de branch sem autorização. Se o plano exigir `main`, confirme que ela é a
branch atual antes de editar.

## Execução marco a marco

Para cada marco:

### 1. Abrir o marco

- releia objetivo, abrangência, tarefas, validações, fechamento e DoD;
- confirme que os marcos dependentes já têm commit local;
- inspecione novamente os arquivos e callers que serão modificados;
- identifique o menor corte coerente que entrega todo o marco.

### 2. Implementar tarefas

- execute os itens `TAR` na ordem de dependência;
- mantenha contratos explícitos e ownership nas camadas corretas;
- atualize todos os callers no mesmo hard cut;
- remova código, testes, fixtures e docs obsoletos no próprio marco;
- escreva ou atualize testes junto da mudança de comportamento;
- não marque `TAR` apenas porque um arquivo foi criado: confirme o resultado
  observável e os callers;
- se a realidade exigir corrigir o plano, preserve IDs existentes, atualize a
  descrição e registre a base técnica da mudança.

### 3. Validar durante o desenvolvimento

- execute testes focados após cada grupo coerente;
- corrija falhas causadas pela mudança e falhas preexistentes que bloqueiem a
  comprovação do marco;
- rode `make ci-fast` ou o gate rápido real definido pelo plano;
- valide casos “Dado/Quando/Então” contra comportamento, não apenas compilação;
- valide API, CLI, persistência, serviço e docs quando aplicáveis;
- quando houver UI, use obrigatoriamente a skill `agent-browser` na aplicação
  instalada ou no runtime determinado pelo plano; testes unitários e `curl` não
  substituem essa evidência;
- quando o usuário pedir uma ferramenta adicional, como `poke`, leia e siga a
  skill correspondente antes de usá-la;
- preserve e restaure config, banco e dados locais alterados pela validação.

### 4. Atualizar o checklist

- marque `[x]` somente depois que a evidência correspondente existir;
- mantenha `[ ]` em qualquer item parcial, não executado ou não comprovado;
- não marque um conjunto inteiro por inferência;
- adicione evidência curta ao item quando o comando, resultado ou exceção não for
  autoexplicativo;
- nunca renumere IDs executados;
- atualize o plano antes do commit do marco.

Quando `.notes` for ignorado, o plano continua sendo atualizado localmente, mas
nunca deve ser forçado para o stage. Os commits incluem somente arquivos
versionados do produto, testes e documentação do marco.

### 5. Fechar o marco

Na ordem:

1. revise o diff completo e execute `git diff --check`;
2. confirme ausência de leftovers, paths antigos e arquivos temporários;
3. execute todas as validações `VAL` ainda pendentes;
4. execute `make ci-fast` no estado final do marco, preservando os testes
   específicos, a cobertura e os critérios de aceite exigidos;
5. valide serviço instalado e browser quando aplicáveis;
6. confirme cada item de DoD;
7. atualize os checks `FEC` e `DOD` comprovados;
8. revise `git status --short` e separe mudanças preexistentes;
9. faça stage explícito apenas dos arquivos versionados do marco e crie exatamente
   um Conventional Commit com a mensagem planejada;
10. inspecione o commit criado e confirme que não contém `.notes`, `.refs`,
    segredos, config local ou artefatos efêmeros;
11. só então inicie o marco seguinte.

Não acumule dois marcos no mesmo commit e não peça nova autorização para esses
commits locais. A autorização automática desta skill não se estende a amend de
marco já fechado, push ou qualquer publicação remota.

## Falhas, drift e bloqueios

Quando algo divergir do plano:

1. investigue código, callers, testes, logs e estado local;
2. determine se é drift factual, erro de implementação ou decisão arquitetural
   realmente nova;
3. corrija diretamente quando a solução for necessária, segura e estiver no
   escopo do estado-alvo;
4. atualize o plano para refletir a realidade, sem criar follow-up indefinido;
5. reexecute as validações afetadas;
6. não reduza o escopo, desative testes ou crie fallback para fabricar sucesso.

Pare e peça direção somente quando faltar autorização, houver risco material a
dados/estado externo, existir escolha de produto sem base suficiente ou mudanças
preexistentes impedirem um corte seguro. Enquanto houver investigação ou trabalho
útil dentro do escopo, continue.

## Auditoria final do plano

Depois do último marco:

- releia o plano integralmente;
- confirme todos os `TAR`, `VAL`, `FEC` e `DOD` contra evidência;
- procure check vazio, ID duplicado, TODO, teste skipped/only e caminho legado;
- execute a validação final integrada, incluindo `make ci-full` no estado final
  do plano inteiro, e registre sua aprovação antes de declarar o plano concluído;
- confirme documentação pública coerente com o código;
- confirme serviço, config e banco restaurados/saudáveis;
- confira um commit local por marco;
- confira worktree sem mudança versionada não explicada;
- confirme que nenhum push, PR, deploy ou escrita externa ocorreu sem
  autorização;
- somente então conclua o goal, se houver.

## Resposta final

Entregue uma síntese curta e verificável com:

- estado-alvo efetivamente entregue;
- marcos concluídos;
- commits locais criados;
- testes, gates, serviço e browser validados;
- arquivos ou áreas arquiteturais principais;
- estado final do worktree;
- confirmação explícita de push feito ou não feito;
- qualquer limitação real ainda existente.

Se algum item obrigatório permanecer aberto, não chame o plano de concluído e não
complete o goal.
