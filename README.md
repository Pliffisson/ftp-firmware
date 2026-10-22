# Firmware de equipamentos

[![Docker Compose](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/compose/)
[![Linux](https://img.shields.io/badge/Host-Linux-FCC624?logo=linux&logoColor=black)](https://www.kernel.org/)
![TFTP UDP 69](https://img.shields.io/badge/TFTP-UDP%2069-2563EB)
![Dashboard TCP 8088](https://img.shields.io/badge/Dashboard-TCP%208088-16A34A)
![Python 3.12](https://img.shields.io/badge/Python-3.12-3776AB?logo=python&logoColor=white)
![Go 1.24](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)

Servidor TFTP em contêiner para distribuir firmwares a equipamentos na rede local. O dashboard permite enviar arquivos, copiar a URL TFTP, acompanhar o download feito pelo equipamento e excluir firmwares.

## Visão do dashboard

- 📦 **Envio de firmware:** selecione um arquivo de até 2 GB e acompanhe o upload para o servidor.
- 📋 **URL pronta para uso:** copie o endereço TFTP do firmware listado para configurar o equipamento.
- 📈 **Download acompanhado:** veja percentual, volume enviado e IP do equipamento durante a transferência.
- ✅ **Conclusão registrada:** consulte o horário e o último download TFTP concluído, inclusive após recarregar a página.
- 🗑️ **Gestão de arquivos:** exclua um firmware após confirmação.
- 🖼️ **Identidade visual:** o logo é servido pelo host e aparece para todos os navegadores que abrem o dashboard.

Os nomes, IPs, tamanhos e horários mostrados no dashboard dependem dos arquivos e das transferências de cada instalação.

## Requisitos

- Host **Linux** com Docker Engine e Docker Compose.
- Porta **UDP 69** livre para o TFTP. As transferências também usam portas UDP temporárias.
- Porta **TCP 8088** livre para o dashboard, ou outra porta definida em `.env`.
- Rede entre o servidor e os equipamentos que permita tráfego TFTP UDP.

O serviço TFTP usa a rede do host Linux. O dashboard é publicado separadamente na porta HTTP configurada.

## Instalação

Clone o repositório no servidor e, dentro da pasta do projeto, execute:

```sh
docker compose up -d --build
docker compose ps
```

Abra `http://IP_DO_SERVIDOR:8088` em um navegador da rede local. O dashboard não pede login.

Para trocar a porta HTTP, crie um arquivo `.env` a partir do exemplo, edite `DASHBOARD_PORT` e recrie o dashboard:

```sh
cp .env.example .env
# Edite DASHBOARD_PORT em .env.
docker compose up -d dashboard
```

O arquivo `.env` é local e não é enviado ao GitHub.

## Enviar e baixar um firmware

1. No dashboard, selecione o arquivo e clique em **Enviar firmware para o servidor TFTP**. A barra mostra o progresso do envio do navegador para o servidor. O limite é **2 GB por arquivo**; um nome já existente não é substituído.
2. Na lista de arquivos, clique em **Copiar URL TFTP**. A URL usa o endereço pelo qual o dashboard foi aberto, por exemplo `tftp://192.168.1.10/firmware.swu`. Abra o dashboard pelo IP que o equipamento consegue alcançar, não por `localhost`.
3. Configure o equipamento para buscar o firmware usando essa URL. Copiar a URL apenas inicia a espera no dashboard; **o equipamento precisa solicitar o arquivo**.
4. Quando a solicitação TFTP começar, o dashboard mostra o IP do equipamento e o progresso do download. Ao terminar, mostra o volume transferido e o horário. A última transferência concluída continua visível após recarregar a página.

A barra confirma a transferência **TFTP do arquivo**. Ela não acompanha a instalação ou a reinicialização do equipamento. Se não houver solicitação por 10 minutos, a espera no dashboard termina; clique em **Copiar URL TFTP** para acompanhar uma nova tentativa.

Também é possível colocar arquivos diretamente em `firmwares/` no host. Eles ficam disponíveis por TFTP sem reiniciar os contêineres. Arquivos adicionados manualmente precisam ser legíveis pelo serviço, por exemplo:

```sh
chmod 644 firmwares/firmware.swu
```

O botão **Excluir** remove o arquivo de `firmwares/` após confirmação.

## Arquivos e dados locais

| Caminho | Conteúdo | GitHub |
| --- | --- | --- |
| `firmwares/` | Arquivos oferecidos pelo TFTP | Ignorado; os firmwares não são enviados ao repositório |
| `branding/logo.png` | Logo exibido no cabeçalho | Incluído no repositório |
| `transfers/` | Estado da última transferência por firmware | Ignorado |
| `.env` | Porta HTTP personalizada, se usada | Ignorado |

Os três diretórios são montados do host nos contêineres. Os arquivos permanecem após `docker compose down` e após atualizações dos contêineres. Para substituir o logo, troque `branding/logo.png` por outro PNG no host e atualize a página.

Se a pasta ainda não estiver inicializada como repositório Git, execute `git init`. Antes de publicar no GitHub, execute `git add .` e `git status --short`; confirme que nenhum firmware de `firmwares/`, registro de `transfers/` ou arquivo `.env` aparece na lista. O logo e `.env.example` devem aparecer.

## Rede e acesso

O TFTP escuta em **UDP 69** e usa portas UDP adicionais durante cada transferência. O dashboard escuta em **TCP 8088** por padrão. Se houver firewall, permita essas conexões entre a rede dos equipamentos e o servidor. O dashboard não tem autenticação; mantenha sua porta acessível apenas na rede local.

O servidor TFTP oferece os firmwares **somente para leitura**. O dashboard grava os uploads na mesma pasta montada. O progresso é medido no servidor TFTP e pode refletir um dispositivo que iniciou o download mesmo que a URL tenha sido copiada em outro navegador.

## Operação

```sh
docker compose ps                 # Estado dos serviços
docker compose logs -f tftp       # Solicitações e conclusão TFTP
docker compose logs -f dashboard  # Logs do dashboard
docker compose down               # Para os contêineres, sem apagar os arquivos locais
```

Para atualizar o projeto após novas alterações no código:

```sh
docker compose up -d --build
```

Para testar a conectividade de dentro do contêiner TFTP até um equipamento:

```sh
docker compose exec tftp ping -c 4 IP_DO_EQUIPAMENTO
```

Se o dashboard abrir, mas o download não começar, confira o IP usado na URL, a rota entre equipamento e servidor e a liberação do tráfego UDP no firewall. Se o equipamento solicitou o arquivo e a barra parar antes de 100%, consulte `docker compose logs tftp` e tente novamente.
