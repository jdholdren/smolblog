# smolblog
The tiniest site generator I could think of.

## The Idea

This main command takes in a path to a `manifest.json` and serves a site based
on the routes in the JSON.
Each GET call re-reads the manifest and parses layouts so not reloading of the binary is required,
just your browser tab.
It then uses those layouts and manifest to execute the template you specify.
This should allow for rather fast iteration of a site.

When you're ready to export it, you can use the `-output` flag.
This will start the server as normal, but then use `wget` to generate the
static version of your site in the location you specify.
