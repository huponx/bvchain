# bvchain

Two Kubernetes CronJobs for the BV portal, plus a CLI to test them:

| Binary | Role |
| --- | --- |
| `cmd/bvchain` | Logs in to the BV portal, checks in and out the work sessions of today (morning from 08:00, checkout from 12:00 and 17:30). Optional Telegram message after each check-in/check-out; a message when a step fails. |
| `cmd/bvreport` | Logs in, reads the calendar of the current period (21st to 20th), counts the working days and sends the working report to Telegram (only when a day is missing, unless `REPORT_ALWAYS_SEND=true`). |
| `cmd/bvctl` | Manual tool: sends the Telegram messages with sample data (to test templates and the courier wiring) and builds the report of any month. See [bvctl](#bvctl). |

Module: `github.com/huponx/bvchain`. Shared libraries come from `gocommon` (`config`, `logging`, `grpcclient`, `worker`, `retry`, `otp`, `jwtx`); the messages are sent through [courier](../courier) (gRPC, `huponx/protogen` stubs).

```
CronJob (bvchain | bvreport) --HTTPS--> BV portal
        |
        +--gRPC (ServiceAccount token, audience courier)--> courier --> Telegram
```

## Messages

The templates live in courier (`internal/notify/templates/{en,vi}/<id>/telegram.txt.tmpl`):

| `template_id` | Sent by | Data |
|---|---|---|
| `bvchain_checkin`, `bvchain_checkout` | bvchain (`NOTIFICATION_SUCCESS_ENABLED=true`) | `shift` (`1` morning, `2` afternoon), `time` |
| `bvchain_failed` | bvchain, bvreport | `step` (`authentication`, `calendar`, `check-in`, `check-out`), `error` |
| `bvreport_working_report` | bvreport | `from_date`, `to_date`, `today`, `total_in_period`, `total_to_today`, `days_with_cico`, `missing_days` |

## Configuration

All settings are environment variables (`.env.example` lists them; `.env` is git-ignored).

| Variable | Default | Used by | Notes |
| --- | --- | --- | --- |
| `BV_USERNAME`, `BV_PASSWORD`, `BV_OTP_SECRET` | | all | BV account; `BV_OTP_SECRET` is the base32 TOTP secret |
| `COURIER_TARGET` | | all | courier gRPC `host:port` |
| `COURIER_TIMEOUT` | `10s` | all | per call |
| `COURIER_INSECURE` | `true` | all | plaintext gRPC (courier has no TLS of its own inside the cluster) |
| `COURIER_TOKEN_FILE` | | all | projected ServiceAccount token (audience `courier`); re-read when it rotates. Wins over `COURIER_S2S_TOKEN` |
| `COURIER_S2S_TOKEN` | | local | static Bearer token for a local courier started with `ENV=local` |
| `TELEGRAM_CHAT_ID` | | all | chat that receives the messages |
| `NOTIFY_LOCALE` | `vi` | all | template language, `vi` or `en` |
| `DELAY` | `0s` | jobs | random sleep up to this value before the run, so the check-in time varies |
| `NOTIFICATION_SUCCESS_ENABLED` | `false` | bvchain | message after each check-in/check-out |
| `REPORT_ALWAYS_SEND` | `false` | bvreport | send even when no day is missing |
| `LOG_LEVEL`, `LOG_ENCODING`, `SERVICE_NAME`, `ENV` | `info`, `console`, binary name, `dev` | all | `LOG_ENCODING=json` in the cluster, `LOG_LEVEL=debug` logs every calendar day |

## Working days of the report

For the period 21st to 20th (UTC+7) the report counts working days from the calendar items of the API:

1. Items are de-duplicated by date (the first item of a date wins; a `WARN` lists the fields that differ).
2. Dates outside the period are dropped (`WARN`).
3. Each date is classified: weekend (`is_weekend`), holiday (`is_holiday`), leave (`leave_type` or the submitted-leave flags, per shift), working otherwise. A half-day leave counts `0.5`.
4. `total_in_period` is the sum of the working days of the period, `total_to_today` the same up to today (inclusive), `days_with_cico` the part covered by check-in/check-out (`total_working_day` of the API, at most the day's weight), `missing_days` the rest.

The previous implementation counted every item that was not a weekend, so rows returned twice or outside the period, holidays and leave days inflated the count (24 instead of 22 for 21/09 to 20/10). The log line `working days summary` prints the raw item count, duplicates, out-of-period dates, weekends, holidays, leave days and the old count (`legacy_total_in_period`) next to the new totals, so a difference can be traced to its cause. A warning is also logged when `is_weekend` disagrees with the weekday of the date (the API flag is used).

## bvctl

`bvctl` runs from a laptop. It sends through courier, so it needs the same environment as the jobs (copy `.env.example` to `.env`, fill it and load it: `set -a; source .env; set +a`; `make` does not read `.env`).

```bash
make build                                  # bin/bvchain, bin/bvreport, bin/bvctl
bin/bvctl help
# or without building: make bvctl-run ARGS='notify checkin'
```

### Reach courier

In a cluster courier only accepts ServiceAccount tokens listed in `COURIER_AUTH_ALLOWED`. From a laptop:

```bash
kubectl -n application port-forward svc/courier 9091:9090 &
export COURIER_TARGET=localhost:9091
# a token of the ServiceAccount of the job (it is allowed in courier), audience courier
kubectl -n application create token bvchain --audience courier --duration 30m > /tmp/courier-token
export COURIER_TOKEN_FILE=/tmp/courier-token
export TELEGRAM_CHAT_ID=<chat id>
```

Against a local courier (`make run-courier` in the courier repository with `ENV=local`) no token is needed: leave `COURIER_TOKEN_FILE` empty, or set `COURIER_S2S_TOKEN` when the local courier was started with one.

### Send a message with sample data

No login to BV; needs `COURIER_TARGET`, the token and `TELEGRAM_CHAT_ID` (or `-chat`).

```bash
bvctl notify checkin                           # morning shift, time now
bvctl notify checkout -shift 2 -time 17:45     # afternoon shift at 17:45
bvctl notify failed                            # sample failure message
bvctl notify report                            # sample working report
bvctl notify checkin -locale en -chat -100123  # other language and chat
bvctl notify report -data missing_days=03      # override a template value (repeatable)
```

| Flag | Default | |
| --- | --- | --- |
| `-chat` | `TELEGRAM_CHAT_ID` | Telegram chat id |
| `-locale` | `NOTIFY_LOCALE` (`vi`) | `vi` or `en` |
| `-shift` | `1` | `checkin`/`checkout`: `1` morning, `2` afternoon |
| `-time` | now | `checkin`/`checkout`: `HH:MM` shown in the message |
| `-data k=v` | | override a template value (repeatable) |

Every run sends a message (no idempotency key).

### Build and send the real report of a month

Logs in to BV (needs `BV_USERNAME`, `BV_PASSWORD`, `BV_OTP_SECRET`), reads the calendar and counts the working days. `-month` names the period after the month it ends in: `10/2025` is 21/09/2025 to 20/10/2025.

```bash
bvctl report -month 10/2025 -dry-run -verbose   # print the report and the per-day table, send nothing
bvctl report -month 10/2025                     # send the report to TELEGRAM_CHAT_ID
bvctl report                                    # the current period
bvctl report -month 10/2025 -skip-empty         # like the job: send only when a day is missing
```

| Flag | Default | |
| --- | --- | --- |
| `-month MM/YYYY` | current period | period that ends on the 20th of that month |
| `-chat`, `-locale` | as above | |
| `-dry-run` | off | print the report, send nothing (no courier needed) |
| `-verbose` | off | debug log of every calendar day and the per-day table |
| `-skip-empty` | off | do not send when no day is missing (the job's behaviour; `bvctl` sends by default) |

Reading the `-verbose` output: the table has one row per date of the period (`class` is `working`, `weekend`, `holiday` or `leave`; `weight` is the working part of the day, `credit` the part covered by check-in/check-out, `twd` the `total_working_day` of the API, `future` days are not counted in `total_to_today`). After the table come the template values and `legacy count (old report)`, the number the previous implementation would have reported. The `WARN` lines above it name every duplicate, out-of-period date and weekday mismatch.

### Run bvctl in the cluster (kubectl)

The `bvreport` image also contains `/bvctl`. Create a one-off Job from the CronJob and replace the command: the Job keeps the environment, the secrets (`BV_*`, `TELEGRAM_CHAT_ID`), the ServiceAccount and the courier token of the CronJob, so no port-forward or token is needed. Needs `jq` and an image released after `/bvctl` was added.

```bash
NS=application
run() {  # run <job name> <bvctl args...>
  name=$1; shift
  kubectl -n $NS create job "$name" --from=cronjob/bvreport-cronjob --dry-run=client -o json \
    | jq --args '.spec.template.spec.containers[0].command=["/bvctl"] | .spec.template.spec.containers[0].args=$ARGS.positional' -- "$@" \
    | kubectl -n $NS apply -f -
  kubectl -n $NS wait --for=condition=complete --timeout=120s job/"$name" || true
  kubectl -n $NS logs job/"$name"
}

run bvctl-report report -month 10/2025 -dry-run -verbose   # numbers only, nothing is sent
run bvctl-send   report -month 10/2025                     # send the report of 21/09 to 20/10
run bvctl-test   notify checkin                            # sample message
kubectl -n $NS delete job bvctl-report bvctl-send bvctl-test
```

Use a new job name for every run (a Job name cannot be reused until it is deleted), and delete the jobs afterwards as above. Only the `bvreport` image contains `/bvctl`; its ServiceAccount is allowed in courier like the one of `bvchain`.

## Local development

```bash
make test        # go test ./...
make vet
make build
```

`go.mod` pins released versions of `gocommon` and `protogen`. When a change needs an unreleased `gocommon` (the repositories sit side by side), use a workspace instead of a `replace`: `go.work` is git-ignored.

```bash
go work init . ../gocommon        # once; `make tidy` ignores it (GOWORK=off)
```

> This repository uses the `gocommon` packages `retry`, `otp` and `jwtx.ParseUnverified`, available since `gocommon` v0.3.0; `go.mod` pins `v0.3.1`.

## Images and charts

Two images are built from the repository root (distroless, non-root): `build/bvchain/Dockerfile` and `build/bvreport/Dockerfile`.

```bash
make docker IMAGE_TAG=dev     # ghcr.io/huponx/bvchain:dev and ghcr.io/huponx/bvreport:dev
```

Helm charts `charts/bvchain` and `charts/bvreport` wrap the `go-job` chart (dependency alias `bv-job`, so all values sit under `bv-job:`). `version`, `appVersion` and `image.tag` are written at release time by the CI; the committed values only keep `helm template` working. Each chart also creates the ServiceAccount of the job (`bvchain`, `bvreport`, no API token mounted) and the job mounts a projected token for the audience `courier`.

```bash
cd charts/bvchain && helm dependency build && helm lint . -f ci/full-values.yaml
helm template bvchain . -f ci/full-values.yaml
```

Helm replaces lists, so the Application in `mm-deploy` repeats the whole `env` and `secrets` lists. Secret values never go in git: reference existing Secrets (`secretName`/`key`).

### Courier must allow the jobs

courier authorizes callers by ServiceAccount. `COURIER_AUTH_ALLOWED` of courier (`mm-deploy`, `applications/<env>/courier.yaml`) lists `system:serviceaccount:application:bvchain` and `system:serviceaccount:application:bvreport`.

### Secrets

Created by `mm-ansible` (`make deploy TAGS=secrets`, `k8s_secrets.yml` and the vault) in the namespace `application`:

| Secret | Keys | Used for |
|---|---|---|
| `job-bv-credentials` | `bv-password`, `bv-otp-secret` | `BV_PASSWORD`, `BV_OTP_SECRET` |
| `telegram-hung.phamqk-chat` | `chat-id` | `TELEGRAM_CHAT_ID` |

The Telegram bot token is not here: courier's worker sends the messages with its own bot.

## CI/CD

`.github/config/ci.yaml` is the contract and `.github/workflows/{pr,main,staging,production}.yml` call the reusable workflows of `huponx/actions` (`service-*.yml`). Both apps (`bvchain`, `bvreport`) are built, packaged and deployed on every flow:

| Flow | Trigger | Result |
|---|---|---|
| pr | pull request to `main` or `release*` | lint, test, build, Trivy scan, `helm lint`; nothing pushed |
| main | push to `main` | image and chart pushed (`1.YYYYMMDD.<run>-main-<sha7>`), deployment PR for `dev` in `mm-deploy` |
| staging | push to `release*` | pre-release `v<YYYY>.<MM>.N-release-<sha7>`, image and chart pushed, deployment PR for `staging` |
| production | release published | no build; verifies the image and chart of the release tag exist, deployment PR for `production` |

`N` only increases after the previous pre-release of the month was published; until then a new push changes only the short sha. Required secret: `BOT_TOKEN` (write access to `mm-deploy`).

## Setup (one time)

Commands use the [GitHub CLI](https://cli.github.com) (`gh auth login` first, with admin rights on the repositories). Package visibility and access are set in the GitHub UI (package settings). Create the repository first: `gh repo create huponx/bvchain --private --source . --push`.

1. **Publish `huponx/actions`**: merge it and make sure a release exists and the floating `v1` tag points to it (the workflows use `huponx/actions/...@v1`). Other repositories must be allowed to use its workflows:

   ```bash
   gh release list -R huponx/actions --limit 3                     # a release must exist
   gh api repos/huponx/actions/git/ref/tags/v1 --jq .object.sha    # v1 must exist
   # private repo only: share the actions with the other repositories of the owner
   gh api -X PUT repos/huponx/actions/actions/permissions/access -f access_level=user
   ```

2. **Token for `mm-deploy`**: `GITHUB_TOKEN` cannot open PRs in another repository. Pick one:

   ```bash
   # a) PAT (fine-grained, repository huponx/mm-deploy: Contents and Pull requests write)
   gh secret set BOT_TOKEN -R huponx/bvchain                       # paste the token

   # b) GitHub App installed on mm-deploy (Contents and Pull requests write)
   gh variable set DEPLOY_APP_ID -R huponx/bvchain --body "<app id>"
   gh secret set DEPLOY_APP_PRIVATE_KEY -R huponx/bvchain < app-private-key.pem
   ```

   The App takes precedence when `DEPLOY_APP_ID` is set; without it `BOT_TOKEN` is used.

3. **Optional**: Slack notification of deployment PRs (set `slack-channel` in `ci.yaml`):

   ```bash
   gh secret set SLACK_WEBHOOK_URL -R huponx/bvchain
   ```

4. **Workflow permissions** (packages and the staging pre-release need write):

   ```bash
   gh api -X PUT repos/huponx/bvchain/actions/permissions/workflow \
     -f default_workflow_permissions=write -F can_approve_pull_request_reviews=false
   ```

5. **GHCR packages**: after the first `main` run the packages `bvchain`, `bvreport`, `charts/bvchain` and `charts/bvreport` exist. Link each to the repository (package settings, "Manage Actions access", role Write) and give the ArgoCD clusters pull access (public, or an image pull secret and an OCI repository credential in each ArgoCD). `go-job` 1.1.0 must be readable from `oci://ghcr.io/huponx/charts`.

   ```bash
   gh api "/user/packages?package_type=container" --jq '.[].name'   # list the packages (use /orgs/<org>/packages for an org)
   ```

6. **Branch ruleset** for `main` (PR required, squash only, required checks of the `pr` flow, no force push or deletion). Merge the workflows to `main` first, while `main` is unprotected: status checks only show up in the ruleset picker after they have run once. Save as `main-ruleset.json` and apply (the file is only an input, do not commit it):

   ```json
   {
     "name": "protect main",
     "target": "branch",
     "enforcement": "active",
     "conditions": { "ref_name": { "include": ["~DEFAULT_BRANCH"], "exclude": [] } },
     "rules": [
       { "type": "deletion" },
       { "type": "non_fast_forward" },
       { "type": "pull_request", "parameters": {
           "required_approving_review_count": 0, "dismiss_stale_reviews_on_push": false,
           "require_code_owner_review": false, "require_last_push_approval": false,
           "required_review_thread_resolution": false,
           "allowed_merge_methods": ["squash"] } },
       { "type": "required_status_checks", "parameters": {
           "strict_required_status_checks_policy": true,
           "required_status_checks": [
             { "context": "pr / contract" },
             { "context": "pr / lint-test" },
             { "context": "pr / build / build (bvchain)" },
             { "context": "pr / build / build (bvreport)" } ] } }
     ],
     "bypass_actors": []
   }
   ```

   ```bash
   REPO=huponx/bvchain
   gh api repos/$REPO/rulesets --method POST --input main-ruleset.json
   gh api repos/$REPO/rulesets --jq '.[] | {id, name, enforcement}'   # verify
   ```

   Or in the UI: Settings > Rules > Rulesets > New ruleset > Import a ruleset. The check names are `<caller job> / <reusable job>`; confirm them with `gh pr checks <pr>` after the first PR run. To change it later: `gh api repos/$REPO/rulesets/<id> --method PUT --input main-ruleset.json`. Rulesets on a private repo need a paid GitHub plan.

   Second ruleset for `release*`: a merge into it creates a staging pre-release, so it needs a PR with the same checks as `main` (the `pr` flow also runs on PRs into `release*`), and no force push or deletion. Save as `release-ruleset.json` and apply the same way (`gh api repos/$REPO/rulesets --method POST --input release-ruleset.json`):

   ```json
   {
     "name": "protect release branches",
     "target": "branch",
     "enforcement": "active",
     "conditions": { "ref_name": { "include": ["refs/heads/release*"], "exclude": [] } },
     "rules": [
       { "type": "deletion" },
       { "type": "non_fast_forward" },
       { "type": "pull_request", "parameters": {
           "required_approving_review_count": 0, "dismiss_stale_reviews_on_push": false,
           "require_code_owner_review": false, "require_last_push_approval": false,
           "required_review_thread_resolution": false,
           "allowed_merge_methods": ["merge", "squash"] } },
       { "type": "required_status_checks", "parameters": {
           "strict_required_status_checks_policy": false,
           "required_status_checks": [
             { "context": "pr / contract" },
             { "context": "pr / lint-test" },
             { "context": "pr / build / build (bvchain)" },
             { "context": "pr / build / build (bvreport)" } ] } }
     ],
     "bypass_actors": []
   }
   ```

   To update the existing ruleset: `gh api repos/$REPO/rulesets --jq '.[] | {id, name}'`, then `gh api repos/$REPO/rulesets/<id> --method PUT --input release-ruleset.json`. The ruleset applies to every `release*` branch, so create the first one (for example `release`) from `main` before applying it, or push it while the ruleset is not yet active.

7. **`mm-deploy`**:

   ```bash
   gh repo edit huponx/mm-deploy --enable-auto-merge --delete-branch-on-merge
   gh label create automerge -R huponx/mm-deploy --color 0E8A16 --description "Merge automatically when checks pass" || true
   ```

   - required checks on `main`: `ci`, `diff`, `cve`, `pr-title`;
   - each ArgoCD (one per environment) has a root app with `path: applications/<env>`;
   - after the first publish, `applications/<env>/bvchain.yaml` and `bvreport.yaml` already exist for dev, staging and production: set `targetRevision` to the published version (the committed one is a sample); later versions come from the deployment PRs.

8. **Cluster side** (from `mm-ansible`, `inventories/production`):

   ```bash
   # 1. add the values to the vault, keys application-job-bv-credentials (bv-password, bv-otp-secret)
   #    and application-telegram-hung.phamqk-chat (chat-id)
   ansible-vault edit inventories/production/group_vars/all/k8s_secrets_vault.yml
   # 2. create the Secrets (k8s_secrets.yml already lists them)
   make deploy TAGS=secrets
   ```

   - courier must allow the ServiceAccounts: `COURIER_AUTH_ALLOWED` in `applications/<env>/courier.yaml` lists `bvchain` and `bvreport` (already set; sync courier);
   - the courier release that runs must contain the templates `bvchain_*` and `bvreport_*` (the worker renders them).

9. **Environments**: staging and production are `enabled: true` in `.github/config/ci.yaml`; set `enabled: false` for an environment until its ArgoCD and `applications/<env>` exist. For production add `reviewers` and keep `automerge` off.

10. **Verify**:

    ```bash
    gh secret list -R huponx/bvchain && gh variable list -R huponx/bvchain
    # merge a PR into main, then follow the run
    gh run list -R huponx/bvchain --workflow main.yml --limit 1
    gh run watch -R huponx/bvchain
    gh pr list -R huponx/mm-deploy --label automerge                 # the dev deployment PR
    # after the deployment: run the job once and read the log (the CronJob is named <release>-cronjob)
    kubectl -n application create job --from=cronjob/bvreport-cronjob bvreport-manual
    kubectl -n application logs job/bvreport-manual
    ```

    The Telegram message arrives; to test messages without waiting, see [Run bvctl in the cluster](#run-bvctl-in-the-cluster-kubectl).
