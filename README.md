# go-autoconfig
IMAP/SMTP autodiscover feature for Thunderbird, Apple Mail and Microsoft Outlook

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