## How to use Mariner

### Quickstart: A Full Example of API

To demonstrate how to interact with Mariner, here's a step-by-step process
of how to run a small test workflow and hit all the Mariner API endpoints.

1. On your machine, move to directory `testdata/no_input_test`

2. Fetch token using API key

[//]: # (pragma: allowlist secret)

```
echo Authorization: bearer $(curl -d '{"<api_key>": "<replaceme>", "key_id": "<replaceme>"}' -X POST -H "Content-Type: application/json" https://<replaceme>.planx-pla.net/user/credentials/api/access_token | jq .access_token | sed 's/"//g') > auth
```

3. POST the workflow request
```
curl -d "@request_body.json" -X POST -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs
```

4. Check run status
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>/status
```

5. Fetch run logs (includes output json)
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>
```

6. Fetch your run history (list of runIDs)
```
curl -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs
```

7. Cancel a run that's currently in-progress
```
curl -d "@request_body.json" -X POST -H "$(cat auth)" https://<replaceme>.planx-pla.net/ga4gh/wes/v1/runs/<runID>/cancel
```
