# History pagination

`GET /chat/history` and `GET /chat/history/search` page backwards with a keyset
cursor. `GET /chat/history/capabilities` publishes the contract:

```json
{
  "contractVersion": "2026-11-14",
  "pagination": "keyset",
  "ordering": ["timestamp:desc", "id:desc"],
  "maxPageSize": 500
}
```

## Using it

Add `keyset=true` to every request. The first page is exact (`limit` rows, even
when many messages share a timestamp). Read `next_cursor` and pass it back:

```bash
curl -H "apikey: $TOKEN" \
  'http://localhost:8081/chat/history?chat=5511999999999&limit=50&keyset=true'

curl -H "apikey: $TOKEN" \
  'http://localhost:8081/chat/history?chat=5511999999999&limit=50&keyset=true&before=2026-11-14%2002:20:00&before_id=00000000-0000-0000-0000-000000000004'
```

`before_id` is the row's UUID primary key (`id`), not the WhatsApp `message_id`.

## Compatibility

A request with only `before` and no `keyset` keeps the old behaviour: the page
is padded to a whole second and may return more than `limit`, because a
timestamp-only cursor cannot split a second without losing rows. The manager
uses the keyset form.
