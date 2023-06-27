import plotly.express as px
import sys, json

snapshot_data = json.load(sys.stdin)

# XXX: parameterize to use block count as value instead of byte size
# XXX: parameterize on different types of px chart types
# XXX: parameterize on output port so we can serve this from infra

parents = []
names = []
values = []

for key in snapshot_data:
    path = key.split('/')
    name = path[len(path) - 1]
    parent = path[len(path) - 2]
    stats = snapshot_data[key]
    parents.append(parent)
    names.append(name)
    values.append(stats['Size'])

data = dict(names=names, parents=parents, values=values)
print(data)
fig = px.treemap(data, names='names', parents='parents', values='values')
print(fig)
fig.show()
    
