FROM mcr.microsoft.com/dotnet/sdk:6.0

WORKDIR /app

RUN export DEBIAN_FRONTEND=noninteractive \
     && wget -q https://packages.microsoft.com/config/ubuntu/19.04/packages-microsoft-prod.deb && dpkg -i packages-microsoft-prod.deb \
     && apt-get update && apt-get install -y --no-install-recommends apt-utils && apt-get install -y apt-transport-https ca-certificates curl unzip procps gnupg2 software-properties-common lsb-release \
     # functions core tools
     && curl https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor > microsoft.gpg \
     && mv microsoft.gpg /etc/apt/trusted.gpg.d/microsoft.gpg \
     && sh -c 'echo "deb [arch=amd64] https://packages.microsoft.com/debian/$(lsb_release -rs | cut -d'.' -f 1)/prod $(lsb_release -cs) main" > /etc/apt/sources.list.d/dotnetdev.list' \
     && apt-get update && apt-get install -y azure-functions-core-tools-4 \
     # docker
     && curl -fsSL https://download.docker.com/linux/debian/gpg | apt-key add - \
     && add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/debian $(lsb_release -cs) stable" \
     && apt-get update && apt-get install -y docker-ce-cli \
     # gh cli
     && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | gpg --dearmor -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
     && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
     && apt-get update && apt-get install -y gh \
     # az cli
     && curl -sSL https://aka.ms/InstallAzureCLIDeb | bash \
     # bicep
     && az bicep install \
     # nodejs
     && curl -fsSL https://deb.nodesource.com/setup_16.x | bash - \
     && apt-get install -y nodejs \
     # python
     && apt-get install -y --no-install-recommends python3-pip python-dev \
     && python3 -m pip install --upgrade pip \
     && echo 'alias python=python3' >> ~/.bashrc \
     && echo 'alias pip=pip3' >> ~/.bashrc \
     && apt-get clean -y && rm -rf /var/lib/apt/lists/*
# azd
COPY cli/bin/azd-linux-amd64 /usr/bin/azd
COPY NOTICE.txt .