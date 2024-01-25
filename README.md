# SockChat

A KF chat client.

## Usage

1. `cd` to the main module directory (i.e. `sockchat/`). If you don't know how, this program probably isn't for you.

2. `go run . "$COOKIES"` where `$COOKIES` is the raw cookies value from the site's header.

Use the network tab in your browser's dev tools to find the `chat.ws` connection. Your cookies are found in the request header. It looks a little something like this:

``` sh
xf_tfa_trust=VALUE; xf_user=VALUE; xf_emoji_usage=VALUE; xf_csrf=VALUE; xf_session=VALUE
```

If the connection fails, confirm the URL in your `.env` file is up-to-date.

## Features

* Mention users using numerical IDs. When typing a message, any mentions in the form of `@USER_ID` will be replaced with `@USERNAME,` when the user hits TAB. Example: `@160024 stfu` -> `@y a t s, stfu`

    * The ID mention can appear anywhere in the message, and more than 1 can be used at once. If it's not a recognized user ID, it will not be replaced.

<hr>

Donations are always welcome but never expected nor required:

XMR: `8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn`

[Other crypto](https://trocador.app/anonpay/?ticker_to=xmr&network_to=Mainnet&address=8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn&donation=True&description=SockChat+Donation&bgcolor=) ([Tor version](http://trocadorfyhlu27aefre5u7zri66gudtzdyelymftvr4yjwcxhfaqsid.onion/anonpay/?ticker_to=xmr&network_to=Mainnet&address=8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn&donation=True&description=SockChat+Donation&bgcolor=))
