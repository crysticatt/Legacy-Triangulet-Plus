## Triangulet Opener v42

**Changed**
- **Every worker has its own connection to the site.** Before, all workers shared one connection pool, which could limit how many requests were really in flight at once. Now each worker opens its own, like running several copies of the opener. Raise Workers (and At once) and compare your per-minute number; how much faster it gets depends on the site.

**Download**: Windows `TrianguletOpener-Setup-v42.exe`, Mac `TrianguletOpener-Mac-v42.zip`.
