# Switch from rmfakecloud

The switch changes one setting on the tablet. rmhub does not import data
from rmfakecloud: the tablet uploads its library at the first sync. Keep
rmfakecloud and its data unchanged until you are sure that you stay with
rmhub.

## Steps

1. **Back up the library.** Download each document from rmfakecloud with
   rmapi, with a separate rmapi configuration:

   ```console
   $ export RMAPI_HOST=https://rmfakecloud.example.org
   $ export RMAPI_CONFIG=$HOME/.config/rmapi/rmfakecloud.conf
   $ rmapi ls /
   $ rmapi mget -o $HOME/rm-backup /
   ```

   The first `rmapi` command asks for a pairing code from rmfakecloud.

2. **Create the rmhub user** and an owner token, as in
   [Run your first server](../tutorials/first-server.md), steps 3 and 4.

3. **Create a pairing code** with the owner token:

   ```console
   $ rmhub pair
   ```

   The code expires after 5 minutes. Do steps 4 to 6 in that time, or
   make a new code.

4. **Change the upstream of the proxy** on the tablet to rmhub. Write the
   old value down:

   ```yaml
   # configuration file of rmfakecloud-proxy on the tablet
   upstream: https://rmhub.example.org:8443
   ```

5. **Restart the proxy** on the tablet:

   ```console
   # systemctl restart proxy
   ```

6. **Pair the tablet with the code.** The old device token of the tablet
   is from rmfakecloud, and rmhub refuses it. If the tablet does not ask
   for a code, disconnect it from the cloud in its settings, then connect
   again and type the code.

7. **Check the sync.** The first sync uploads the full library from the
   tablet. This takes time for a large library. Then the library shows
   on rmhub:

   ```console
   $ rmhub ls /
   ```

## Way back

To go back to rmfakecloud, set `upstream` in the proxy configuration on
the tablet to its old value and restart the proxy. Pair again with a
code from rmfakecloud if the tablet asks.

**Warning:** changes that you make while the tablet uses rmhub are not
in rmfakecloud. They get there only when the tablet syncs with
rmfakecloud again. A change that an API client made on rmhub and that
the tablet did not sync is lost. Do not delete the rmfakecloud data
before you are sure that you stay with rmhub.
