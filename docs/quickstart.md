# First request

This guide is for the administrator setting up an instance. You need one supported subscription account and a client. It uses Codex as the example.

If you already have a member login, start with [personal keys](api-keys.md) instead.

## 1. Start SubLane

With Docker, Compose, Bash, curl and jq installed, run:

```sh title="Install a local instance"
curl -fsSL https://raw.githubusercontent.com/murongg/SubLane/main/scripts/install.sh | bash
```

The installer creates `./sublane` and will not overwrite an existing directory. You can [review the script](https://github.com/murongg/SubLane/blob/main/scripts/install.sh) before running it.

Open [127.0.0.1:8080](http://127.0.0.1:8080) and create the administrator. If your instance is already running, use its address and skip installation. On a remote server, use an SSH tunnel for initial access; see [deployment](deployment.md) for network setup and HTTPS before sharing it.

**Ready to continue:** you can sign in and open **Accounts**.

## 2. Connect one subscription

In **Accounts**, add a Codex account through browser authorization. After authorizing, copy the complete localhost callback address from the browser address bar back into SubLane. A localhost connection error at this point is expected; copy the address anyway.

Select **Verify connection**, then open **Models** to check the available model IDs. If you prefer credential import or another provider, follow the relevant steps in [Connect an account](providers.md).

**Ready to continue:** the account is enabled, verification succeeds and its model list contains a model you want to use.

## 3. Put the account in a pool

Open **Account groups**, create a group with a name you recognize, select the subscription account, and save. An account pool is simply the set of subscription accounts a key can use.

New accounts are initially **Unassigned**. Creating a pool is required; a default pool is not created automatically. Leave model restrictions off for this first connection.

You are signed in as the administrator, so you can use this enabled pool without creating a personnel team. To give members access later, follow [Share with your team](members.md).

**Ready to continue:** the pool contains your enabled subscription account.

## 4. Create your personal key

Open **API keys**, create a key, name it, and select your pool. Copy the key for your client. Keep the default expiry unless you need a specific deadline.

This is a SubLane gateway key, separate from the provider's subscription credentials. You do not need a resource allowance, percentages, prices or a token budget.

**Ready to continue:** you have a key and a model ID from that pool.

## 5. Connect the client and send a message

Open the key's configuration guide and follow [Codex client configuration](codex.md#client-configuration). Use your instance address ending in `/v1`, your personal key, and a model ID from the pool. For a local instance, the base URL is `http://127.0.0.1:8080/v1`.

Send a short message from the client. Check that a response arrives, then open **Requests** in SubLane to find the call. A successful login or a visible model list alone does not prove generation works.

**Finished:** your client receives a model response and the request is recorded. You can use the gateway now.

## If a step does not work

| Symptom | Check first |
| --- | --- |
| No pool to select | Create an enabled account group and assign the account. Members also need team authorization. |
| No model available | Verify the account connection and check its model list. See [model discovery](models.md) if it remains empty. |
| The client rejects the key | Check the key, its status and the configured instance address. See [personal keys](api-keys.md). |
| A request fails | Inspect the matching request and use [Everyday use](usage.md) to narrow down the cause. |

## Next, only if needed

To add colleagues, continue with [Share with your team](members.md). To inspect calls and usage, see [Everyday use](usage.md). Usage allowances and advanced controls are optional.
