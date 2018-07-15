# Gravity API CLI [![Build Status](https://travis-ci.org/alexhokl/gravity-api.svg?branch=master)](https://travis-ci.org/alexhokl/gravity-api)

A command line tool to interact with Gravity APIs

##### Prerequisites

- [httpstat](https://github.com/davecheney/httpstat)
- [jq](https://stedolan.github.io/jq)

##### Download

- Feel free to download the latest version from [release page](https://github.com/alexhokl/gravity-api/releases), or
- use `go get -u github.com/alexhokl/gravity-api` if you have Go installed

##### Examples

You can treat this as `curl` on steroid.

###### Configure the API to use

```sh
gravity-api configure -u test.gravity.com
```

###### Configure to use a token

```sh
gravity-api configure -t eaoimoinfjanjfgnaw31232
```

###### Show current configuration

```sh
gravity-api configure show
```

###### Login

```sh
gravity-api login -u your-username -p your-password
```

###### Get

```sh
gravity-api get -r /api/booking/stats
```

###### Get with selected response

Query and select only the second item in `Items`.

```sh
gravity-api get -r /api/booking/stats -s .Items[1]
```

###### Get with parameters from a json file

`params.json`
```json
{
    "param1": "str1",
    "param2": 14
}
```

```sh
gravity-api get -r /api/booking/stats -f params.json
```

###### Post with playload from a json file

`playload.json`
```json
{
    "param1": "str1",
    "param2": {
        "subparam1": "str2",
        "subparam2": "str3",
    }
}
```

```sh
gravity-api post -r /api/booking -f playload.json
```
