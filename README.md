# go-autoconfig
IMAP/SMTP autodiscover feature for Thunderbird, Apple Mail and Microsoft Outlook.

You need DNS SRV-record to get work Outlook and Thunderbird:
```
_autodiscover._tcp IN SRV 0 0 443 autoconfig.example.com.
```
Of course `autoconfig.example.com` domain should point to your server with this service. 

### Thunderbird
`GET https://autoconfig.example.com/mail/config-v1.1.xml`

### Apple Mail
`GET https://autoconfig.example.com/email.mobileconfig?email=your@email.com`

### Outlook
`POST https://autoconfig.example.com/autodiscover/autodiscover.xml`

## Installation
1. Compile (`go mod tidy && go build -o server`) or download binary from releases tab.
2. Edit config.yml and download `templates` directory. It should be with `server` binary.
3. Launch it `./server -config config.yml`.
4. Optionally use `nginx` as reverse-proxy and `systemd` to do daemon.
