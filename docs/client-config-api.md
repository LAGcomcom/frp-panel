# Client Config API

Use a user's API key to retrieve all currently available FRPC client configurations for that account.

## Request

```http
GET /api/client/configs
X-API-Key: <USER_API_KEY>
```

Bearer authentication is also supported:

```http
Authorization: Bearer <USER_API_KEY>
```

Do not put the API key in the query string. Query-string keys are rejected because URLs are commonly stored in access logs and browser history.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "generatedAt": "2026-07-18T08:00:00Z",
    "configs": [
      {
        "serverId": 1,
        "serverName": "node-a",
        "frpVersion": "0.68.0",
        "serverAddr": "203.0.113.10",
        "serverPort": 7000,
        "auth": {
          "method": "token",
          "token": "<USER_API_KEY>"
        },
        "transport": {
          "tcpMux": true
        },
        "metadatas": {
          "apikey": "<USER_API_KEY>",
          "server_id": "1"
        },
        "proxies": [
          {
            "name": "remote-desktop",
            "type": "tcp",
            "localIP": "127.0.0.1",
            "localPort": 3389,
            "remotePort": 6000
          }
        ]
      }
    ]
  }
}
```

Only enabled proxies owned by the API-key account are returned. The response also applies the account's current node-group access rules and excludes unavailable nodes. FRPS node tokens, SSH credentials, other users, and disabled proxies are never returned.
