## Lotus Kibana Dashboard

This folder contains configuration files to create Kibana dashboards to track peer scores and block propagation
throughout Filecoin network.

### Importing dashboard

The peer score and block propagation dashboard configuration is imported via Kibana import saved object [functionality](https://www.elastic.co/guide/en/kibana/current/managing-saved-objects.html#managing-saved-objects-export-objects).

The index patterns will be created automatically when importing dashboards.

#### Custom index

By default, the dashboards target `lotus-pubsub` index which is the default one when running node. The index can be customised via edit on dashboard visualizations.
