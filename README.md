# mattermost-plugin

## To build the plugin as of 8/27/24:

```
go build -o plugin.exe plugin.go
# or if you are on mac, run this instead
GOOS=linux GOARCH=amd64 go build -o plugin.exe plugin.go
tar -czvf plugin.tar.gz plugin.exe plugin.json 
```

## To deploy Mattermost on Docker:

In a terminal window, clone the repository and enter the directory.
```
git clone https://github.com/mattermost/docker
cd docker
```

Create your .env file by copying and adjusting the env.example file.
```
cp env.example .env
```

## Important

At a minimum, you must edit the DOMAIN value in the .env file to correspond to the domain for your Mattermost server.

## Create the required directories and set their permissions.
```
mkdir -p ./volumes/app/mattermost/{config,data,logs,plugins,client/plugins,bleve-indexes}
sudo chown -R 2000:2000 ./volumes/app/mattermost (skip this if on a mac)
```

## If you want to be able to upload plugins via Mattermost GUI:

Go to ./docker/volumes/app/mattermost/config/config.json and change EnablePlugins to true

## Configure TLS for NGINX (optional). If you’re not using the included NGINX reverse proxy, you can skip this step.

### If creating a new certificate and key:
```
bash scripts/issue-certificate.sh -d <YOUR_MM_DOMAIN> -o ${PWD}/certs
```
To include the certificate and key, uncomment the following lines in your .env file and ensure they point to the appropriate files.
```
#CERT_PATH=./certs/etc/letsencrypt/live/${DOMAIN}/fullchain.pem
#KEY_PATH=./certs/etc/letsencrypt/live/${DOMAIN}/privkey.pem
```
If using a pre-existing certificate and key:
```
mkdir -p ./volumes/web/cert
cp <PATH-TO-PRE-EXISTING-CERT>.pem ./volumes/web/cert/cert.pem
cp <PATH-TO-PRE-EXISTING-KEY>.pem ./volumes/web/cert/key-no-password.pem
```
To include the certificate and key, ensure the following lines in your .env file points to the appropriate files.
```
CERT_PATH=./volumes/web/cert/cert.pem
KEY_PATH=./volumes/web/cert/key-no-password.pem
```
## Configure SSO with GitLab (optional). If you want to use SSO with GitLab, and you’re using a self-signed certificate, you have to add the PKI chain for your authority. This is required to avoid the Token request failed: certificate signed by unknown authority error.

To add the PKI chain, uncomment this line in your .env file, and ensure it points to your pki_chain.pem file:
```
#GITLAB_PKI_CHAIN_PATH=<path_to_your_gitlab_pki>/pki_chain.pem
```
Then uncomment this line in your docker-compose.yml file, and ensure it points to the same pki_chain.pem file:
```
# - ${GITLAB_PKI_CHAIN_PATH}:/etc/ssl/certs/pki_chain.pem:ro
```
## Deploy Mattermost.

Without using the included NGINX:
```
sudo docker compose -f docker-compose.yml -f docker-compose.without-nginx.yml up -d
```
To access your new Mattermost deployment, navigate to http://<YOUR_MM_DOMAIN>:8065/ in your browser.

To shut down your deployment:
```
sudo docker compose -f docker-compose.yml -f docker-compose.without-nginx.yml down
```
## Using the included NGINX:
```
sudo docker compose -f docker-compose.yml -f docker-compose.nginx.yml up -d
```
To access your new Mattermost deployment via HTTPS, navigate to https://<YOUR_MM_DOMAIN>/ in your browser.

## To shut down your deployment:
```
sudo docker compose -f docker-compose.yml -f docker-compose.nginx.yml down
```
