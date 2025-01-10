## Lotus Kibana Dashboard

This folder contains configuration files to create Kibana dashboards to track peer scores and block propagation
throughout Filecoin network.

### Importing index template

Index template needs to be imported into Elasticsearch for score weights and to
prevent Elasticsearch from inferring wrong field type.

The [template](./index-template.json) is loaded via [Kibana Index Management](https://www.elastic.co/guide/en/elasticsearch/reference/current/index-mgmt.html) and pasted
into newly created Index Template.

### Importing dashboard

The peer score and block propagation dashboard configuration is imported via Kibana import saved object [functionality](https://www.elastic.co/guide/en/kibana/current/managing-saved-objects.html#managing-saved-objects-export-objects).

#### Custom index

By default, the dashboards and index template target `lotus-pubsub` index which is the default one when running node. The index can be customised via edit on dashboard visualizations and also index template needs to be updated to target new index.
