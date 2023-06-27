import plotly.express as px
import sys, json
import pathlib

snapshot_data = json.load(sys.stdin)

# XXX: parameterize to use block count as value instead of byte size
# XXX: parameterize on different types of px chart types
# XXX: parameterize on output port so we can serve this from infra

parents = []
names = []
values = []

for key in snapshot_data:
    path = pathlib.Path(key)
    name = key
    parent = str(path.parent)
    if key == '/':
        parent = ''
    stats = snapshot_data[key]
    parents.append(parent)
    names.append(name)
    values.append(stats['Size'])

data = dict(names=names, parents=parents, values=values)
print(data)
fig = px.treemap(data, names='names', parents='parents', values='values')
print(fig)
fig.show()
    
