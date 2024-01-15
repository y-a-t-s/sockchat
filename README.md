# SockChat

A KF chat client. Still a bit rough around the edges, but it works.

## Usage

1. `cd` to the main module directory (i.e. `sockchat/`). If you don't know how, this program probably isn't for you.

2. `go run . "$COOKIES"` where `$COOKIES` is the raw cookies value from the site's header.

Use the network tab in your browser's dev tools to find the `chat.ws` connection. Your cookies are found in the request header. It looks a little something like this:

``` sh
xf_tfa_trust=VALUE; xf_user=VALUE; xf_emoji_usage=VALUE; xf_csrf=VALUE; xf_session=VALUE
```

If the connection fails, confirm the URL in your `.env` file is up-to-date.
