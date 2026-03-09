# Midterm Mystery Submission - Hummingbird API

Repository: https://github.com/modimansi/humming-bird-midterm

## Scope Completed


I implemented fixes for all 4 required tickets and the bonus challenge root cause.

This report documents both the functional bug and the underlying engineering issue for each ticket. The goal was not only to patch behavior, but to ensure each fix aligns with the intended API contract and avoids repeat failures in production.

## Ticket #1 - Server started on wrong port

Bug report:
- Server started with `APP_PORT` missing and logged `listening on port undefined`.

Root cause:
- File: `server.js`
- Broken line: `const port = process.env.APP_PORT;` (around line 35)
- Why broken: no fallback value, so `undefined` can be passed to `app.listen(...)`.

Fix:
```diff
-const port = process.env.APP_PORT;
+const port = process.env.APP_PORT || 9000;
```

Result:
- API now defaults to port `9000` when `APP_PORT` is unset.

Why this matters:
- This makes startup behavior deterministic across environments and prevents silent misconfiguration from breaking health checks or routing.

## Ticket #2 - Width is missing from metadata

Bug report:
- Upload with `?width=800`, then `GET /v1/media/:id` does not include `width`.

Root cause:
- File: `clients/dynamodb.js`
- Broken area: `getMedia()` return object (around lines 78-85)
- Why broken: `createMedia()` stores `width`, but `getMedia()` never returns it.

Fix:
```diff
 return {
   mediaId,
   size: Number(Item.size.N),
   name: Item.name.S,
   mimetype: Item.mimetype.S,
   status: Item.status.S,
+  width: Number(Item.width.N),
 };
```

Result:
- `GET /v1/media/:id` now includes numeric `width`.

Why this matters:
- The metadata response now accurately reflects what is stored in DynamoDB, which keeps downstream clients and UI behavior consistent.

## Ticket #3 - Redirect URL is broken (missing scheme)

Bug report:
- `GET /download` while processing returns `202`, but `Location` header lacks `http://`.

Root cause:
- File: `controllers/media.js`
- Broken line: `res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);` (around line 111)
- Why broken: `req.hostname` alone does not include URI scheme or port, so header is not a valid absolute URL.

Fix:
```diff
-res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);
+res.set('Location', `http://${req.get('host')}/v1/media/${mediaId}/status`);
```

Result:
- `Location` header now has full absolute URL format with host/port.

Why this matters:
- Clients that rely on standard HTTP redirect handling can now follow the response reliably without custom URL reconstruction.

## Ticket #4 - Download never redirects even when COMPLETE

Bug report:
- `GET /status` shows `COMPLETE`, but `GET /download` keeps returning `202` forever.

Root cause:
- File: `controllers/media.js`
- Broken line: `if (media.status !== MEDIA_STATUS.PROCESSING)` (around line 108)
- Why broken: this condition returns `202` for any status other than `PROCESSING`, including `COMPLETE`. Redirect branch becomes unreachable for completed media.

Fix:
```diff
-if (media.status !== MEDIA_STATUS.PROCESSING) {
+if (media.status !== MEDIA_STATUS.COMPLETE) {
```

Result:
- `GET /download` now returns `302` redirect when media status is `COMPLETE`.

Why this matters:
- The endpoint behavior now matches the documented lifecycle: non-complete media returns `202`, while complete media returns a downloadable redirect.

## Bonus Challenge - Status never changes

Observation:
- No runtime error, but status updates appear not to affect the same item.

Root cause:
- File: `clients/dynamodb.js`
- Broken line in `setMediaStatus()`: `SK: { S: 'metadata' }` (lowercase)
- Why broken: `createMedia()` and `getMedia()` use `SK = 'METADATA'` (uppercase). `UpdateItem` with lowercase key targets a different/non-existent item and does not fail without a `ConditionExpression`.

Fix:
```diff
-  SK: { S: 'metadata' },
+  SK: { S: 'METADATA' },
```

Additional consistency fix:
```diff
-logger.info({ mediaId, sk: 'metadata', newStatus }, 'Updating media status in DynamoDB');
+logger.info({ mediaId, sk: 'METADATA', newStatus }, 'Updating media status in DynamoDB');
```

Result:
- `setMediaStatus()` now updates the same record created/read by the rest of the app.

Why this matters:
- All DynamoDB operations now target one canonical key, so status transitions are visible to both `/status` and `/download` as intended.

## Consolidated Code Diff

```diff
diff --git a/server.js b/server.js
@@
-const port = process.env.APP_PORT;
+const port = process.env.APP_PORT || 9000;

diff --git a/clients/dynamodb.js b/clients/dynamodb.js
@@
     return {
       mediaId,
       size: Number(Item.size.N),
       name: Item.name.S,
       mimetype: Item.mimetype.S,
       status: Item.status.S,
+      width: Number(Item.width.N),
     };
@@
-      SK: { S: 'metadata' },
+      SK: { S: 'METADATA' },
@@
-    logger.info({ mediaId, sk: 'metadata', newStatus }, 'Updating media status in DynamoDB');
+    logger.info({ mediaId, sk: 'METADATA', newStatus }, 'Updating media status in DynamoDB');

diff --git a/controllers/media.js b/controllers/media.js
@@
-    if (media.status !== MEDIA_STATUS.PROCESSING) {
+    if (media.status !== MEDIA_STATUS.COMPLETE) {
@@
-      res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);
+      res.set('Location', `http://${req.get('host')}/v1/media/${mediaId}/status`);
```

## Verification Commands (run in AWS environment)

```bash
# health
curl http://<alb-dns>/health

# ticket 2
curl -X POST "http://<alb-dns>/v1/media/upload?width=800" -F "file=@sample.jpg"
curl http://<alb-dns>/v1/media/<mediaId>

# ticket 3 + 4
curl -i http://<alb-dns>/v1/media/<mediaId>/download
curl -X PUT "http://<alb-dns>/v1/media/<mediaId>/resize?width=500"
curl http://<alb-dns>/v1/media/<mediaId>/status
curl -i http://<alb-dns>/v1/media/<mediaId>/download

# logs
aws logs tail /ecs/hummingbird/production/api --follow
```

Expected checks:
- Port defaults to `9000` with missing `APP_PORT`.
- `GET /v1/media/:id` includes `width`.
- 202 response includes absolute `Location` URL beginning with `http://`.
- When status is `COMPLETE`, `GET /download` returns `302`.

Observed output (captured during CloudShell code verification):
```bash
$ grep -n "APP_PORT || 9000" server.js
35:const port = process.env.APP_PORT || 9000;

$ grep -n "width: Number(Item.width.N)" clients/dynamodb.js
84:      width: Number(Item.width.N),

$ grep -n "SK: { S: 'METADATA' }" clients/dynamodb.js
29:      SK: { S: 'METADATA' },
62:      SK: { S: 'METADATA' },
110:      SK: { S: 'METADATA' },
155:      SK: { S: 'METADATA' },
186:      SK: { S: 'METADATA' },

$ grep -n "media.status !== MEDIA_STATUS.COMPLETE" controllers/media.js
108:    if (media.status !== MEDIA_STATUS.COMPLETE) {

$ grep -n "http://\${req.get('host')}/v1/media/\${mediaId}/status" controllers/media.js
111:      res.set('Location', `http://${req.get('host')}/v1/media/${mediaId}/status`);
```

Observed output (diff confirmation):
```bash
$ git diff -- server.js clients/dynamodb.js controllers/media.js
diff --git a/clients/dynamodb.js b/clients/dynamodb.js
index 700494c..1613483 100644
--- a/clients/dynamodb.js
+++ b/clients/dynamodb.js
@@ -81,6 +81,7 @@ const getMedia = async (mediaId) => {
       name: Item.name.S,
       mimetype: Item.mimetype.S,
       status: Item.status.S,
+      width: Number(Item.width.N),
@@ -151,7 +152,7 @@ const setMediaStatus = async ({ mediaId, newStatus }) => {
-      SK: { S: 'metadata' },
+      SK: { S: 'METADATA' },
@@ -163,7 +164,7 @@ const setMediaStatus = async ({ mediaId, newStatus }) => {
-    logger.info({ mediaId, sk: 'metadata', newStatus }, 'Updating media status in DynamoDB');
+    logger.info({ mediaId, sk: 'METADATA', newStatus }, 'Updating media status in DynamoDB');
diff --git a/controllers/media.js b/controllers/media.js
index 6bb1c50..8f3b61f 100644
--- a/controllers/media.js
+++ b/controllers/media.js
@@ -105,10 +105,10 @@ const downloadController = async (req, res) => {
-    if (media.status !== MEDIA_STATUS.PROCESSING) {
+    if (media.status !== MEDIA_STATUS.COMPLETE) {
       const SIXTY_SECONDS = 60;
       res.set('Retry-After', SIXTY_SECONDS);
-      res.set('Location', `${req.hostname}/v1/media/${mediaId}/status`);
+      res.set('Location', `http://${req.get('host')}/v1/media/${mediaId}/status`);
diff --git a/server.js b/server.js
index 454790a..46ce716 100644
--- a/server.js
+++ b/server.js
@@ -32,7 +32,7 @@ app.get('/health', (req, res) => {
-const port = process.env.APP_PORT;
+const port = process.env.APP_PORT || 9000;
```


