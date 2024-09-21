# SockChat

A KF chat client.

## Basic Usage

* Windows: `sockchat_ARCH.exe --cookies="$COOKIES"`

* macOS: `./sockchat_macos --cookies="$COOKIES"`

* Linux: `./sockchat_linux_ARCH --cookies="$COOKIES"` 

Where `$COOKIES` is the raw cookies value from the site's header.

### Cookies

Use the network tab in your browser's dev tools to find the `chat.ws` connection on the forum's homepage. Your cookies are found in the request header. It looks a little something like this:

``` sh
xf_tfa_trust=VALUE; xf_user=VALUE; xf_csrf=VALUE; xf_session=VALUE
```

You don't need all of these values to be present. Whatever your browser uses to connect and log you in will work fine.

If the connection fails, confirm the URL in your `config.json` file is up-to-date.

## Notable Features

* Mention users using numerical IDs. When typing a message, any mentions in the form of `@USER_ID` will be replaced with `@USERNAME,` when you hit TAB. Example: `@160024 stfu` -> `@y a t s, stfu`

    * The ID mention can appear anywhere in the message, and more than 1 can be used at once. If it's not a recognized user ID, it will not be replaced.
    
* Tor support.

* Notifications when you're mentioned.

* Lurker mode (Press Shift-Tab).

Read the [wiki](https://github.com/y-a-t-s/sockchat/wiki/Configuration) to learn how to configure these features. It's pretty straightforward and is important to know.

<hr>

Donations are always appreciated but never expected nor required:

XMR: `8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn`

[Other crypto](https://trocador.app/anonpay/?ticker_to=xmr&network_to=Mainnet&address=8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn&donation=True&description=SockChat+Donation&bgcolor=) ([Tor version](http://trocadorfyhlu27aefre5u7zri66gudtzdyelymftvr4yjwcxhfaqsid.onion/anonpay/?ticker_to=xmr&network_to=Mainnet&address=8BjCARiV2uB2gZTbbiMUetfRxcAYZgVM5fXxjEbpmb2nAu8ND1grazZ1EhMGdRqVerAtvEJeiy7SzA3SLXpg2CtRDtCAFfn&donation=True&description=SockChat+Donation&bgcolor=))
