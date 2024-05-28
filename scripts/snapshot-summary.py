import plotly.express as px
import sys, json
import pathlib

snapshot_data = json.load(sys.stdin)

# Possible extensions:
# 1. parameterize to use block count as value instead of byte size
# 2. parameterize on different types of px chart types
# 3. parameterize on output port so we can serve this from infra

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
fig = px.treemap(data, names='names', parents='parents', values='values')
fig.show()
    
