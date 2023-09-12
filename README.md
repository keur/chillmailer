# Chill Mailer

A simple emailing list manager. Supports subscribe, unsubscribe, message
queueing and cancellation.

### Running

The following environment variables should be set

```
ADMIN_PASS=admin_panel_pass # HTTP basic auth for accessing the admin panel
SMTP_HOST=your-mail-server.com
SMTP_PORT=465
SMTP_USER=smtp_username
SMTP_PASS=smtp_password
MX_DOMAIN=segfault.fun # emails will orignate from chillmailer-list@MX_DOMAIN
```

### Admin Panel

The Admin panel supports creating new mailing lists, provides metadata about
existing lists, and shows you who is subscribed. And most importantly, you can
send email blasts to your subscribers.


![List Display](https://i.fluffy.cc/xMKkXpt7BDhKq431KtNdv9knJTTMtwwb.png)
![Draft Email Blast](https://i.fluffy.cc/BCRK5Ql3N3nvHBKDn9n2JQbFbTC1GZdq.png)

### Routes

#### Subscribe

```
POST /unsubscribe
  -H "Content-Type: application/x-www-form-urlencoded"
  -d "list=Blog&email=mail@example.com"
```

#### Unsubscribe

```
GET /unsubscribe/{listName}/{email}/{unsubToken}
```

Note that unsubscribe links are included in every email.
