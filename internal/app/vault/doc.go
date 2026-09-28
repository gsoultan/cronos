/*
Package vault keeps the secrets a project's people store: a Mapbox token typed
into the map builder, a warehouse password typed into the datasource wizard.

Before this a secret was the deployment's alone — an environment variable or a
mounted file, set by whoever ran the process — so connecting a warehouse from
the portal produced a definition naming ${secret:warehouse_password} and
nothing anywhere to answer it. The source sat in the catalogue unopenable until
somebody with a shell set a variable and restarted the server.

Values are sealed before they reach the store (see secret.Sealer) and are never
returned: a secret is written, used and replaced. What anybody can read back is
a name, when it was set and by whom, and which definitions use it.

Control plane. One Service per project, resolving against that project's rows
and, behind them, the deployment's own secrets as far as the deployment shares
them — see secret.Only.
*/
package vault
