import param
import panel as pn
import toml
from .util import get_manifest, print_err


def value_dict(parameterized, renames=None, stringify=False):
    d = dict()
    if renames is None:
        renames = dict()
    for name, p in parameterized.param.objects().items():
        if name == 'name':
            continue
        if name in renames:
            name = renames[name]
        val = p.__get__(parameterized, type(p))
        if isinstance(val, param.Parameterized):
            try:
                val = val.to_dict()
            except:
                val = value_dict(val, renames=renames)
        if stringify:
            val = str(val)
        d[name] = val
    return d


def make_group_params_class(testcase):
    """Returns a subclass of param.Parameterized whose params are defined by the
    'params' dict inside of the given testcase dict"""
    tc_params = dict()
    for name, p in testcase.get('params', {}).items():
        tc_params[name] = make_param(p)

    name = 'Test Params for testcase {}'.format(testcase.get('name', ''))
    cls = param.parameterized_class(name, tc_params, GroupParamsBase)
    return cls


def make_param(pdef):
    """
    :param pdef: a parameter definition dict from a testground plan manifest
    :return: a param.Parameter that has the type, bounds, default value, etc from the definition
    """
    typ = pdef['type'].lower()
    if typ == 'int':
        return num_param(pdef, cls=param.Integer)
    elif typ == 'float':
        return num_param(pdef)
    elif typ.startswith('bool'):
        return bool_param(pdef)
    else:
        return str_param(pdef)


def num_param(pdef, cls=param.Number):
    lo = pdef.get('min', None)
    hi = pdef.get('max', None)
    bounds = (lo, hi)
    if lo == hi and lo is not None:
        bounds = None

    default_val = pdef.get('default', None)
    if default_val is not None:
        if cls == param.Integer:
            default_val = int(default_val)
        else:
            default_val = float(default_val)
    return cls(default=default_val, bounds=bounds, doc=pdef.get('desc', ''))


def bool_param(pdef):
    default_val = str(pdef.get('default', 'false')).lower() == 'true'
    return param.Boolean(
        doc=pdef.get('desc', ''),
        default=default_val
    )


def str_param(pdef):
    return param.String(
        default=pdef.get('default', ''),
        doc=pdef.get('desc', ''),
    )


class Base(param.Parameterized):
    @classmethod
    def from_dict(cls, d):
        return cls(**d)

    def to_dict(self):
        return value_dict(self)


class GroupParamsBase(Base):
    def to_dict(self):
        return value_dict(self, stringify=True)


class Metadata(Base):
    composition_name = param.String()
    author = param.String()

    @classmethod
    def from_dict(cls, d):
        d['composition_name'] = d.get('name', '')
        del d['name']
        return Metadata(**d)

    def to_dict(self):
        return value_dict(self, {'composition_name': 'name'})


class Global(Base):
    plan = param.String()
    case = param.Selector()
    builder = param.String()
    runner = param.String()

    # TODO: link to instance counts in groups
    total_instances = param.Integer()
    # TODO: add ui widget for key/value maps instead of using Dict param type
    build_config = param.Dict(default={}, allow_None=True)
    run_config = param.Dict(default={}, allow_None=True)

    def set_manifest(self, manifest):
        if manifest is None:
            return
        print('manifest:', manifest)
        self.plan = manifest['name']
        cases = [tc['name'] for tc in manifest['testcases']]
        self.param['case'].objects = cases
        print('global config updated manifest. cases:', self.param['case'].objects)
        if len(cases) != 0:
            self.case = cases[0]

        if 'defaults' in manifest:
            print('manifest defaults', manifest['defaults'])
            if self.builder == '':
                self.builder = manifest['defaults'].get('builder', '')
            if self.runner == '':
                self.runner = manifest['defaults'].get('runner', '')


class Resources(Base):
    memory = param.String(allow_None=True)
    cpu = param.String(allow_None=True)


class Instances(Base):
    count = param.Integer(allow_None=True)
    percentage = param.Number(allow_None=True)


class Dependency(Base):
    module = param.String()
    version = param.String()


class Build(Base):
    selectors = param.List(class_=str, allow_None=True)
    dependencies = param.List(allow_None=True)


class Run(Base):
    artifact = param.String(allow_None=True)
    test_params = param.Parameter(instantiate=True)

    def __init__(self, params_class=None, **params):
        super().__init__(**params)
        if params_class is not None:
            self.test_params = params_class()

    @classmethod
    def from_dict(cls, d, params_class=None):
        return Run(artifact=d.get('artifact', None), params_class=params_class)

    def panel(self):
        return pn.Column(
            self.param['artifact'],
            pn.Param(self.test_params)
        )


class Group(Base):
    id = param.String()
    instances = param.Parameter(Instances(), instantiate=True)
    resources = param.Parameter(Resources(), allow_None=True, instantiate=True)
    build = param.Parameter(Build(), instantiate=True)
    run = param.Parameter(Run(), instantiate=True)

    def __init__(self, params_class=None, **params):
        super().__init__(**params)
        if params_class is not None:
            self.run = Run(params_class=params_class)
        self._set_name(self.id)

    @classmethod
    def from_dict(cls, d, params_class=None):
        return Group(
            id=d['id'],
            resources=Resources.from_dict(d.get('resources', {})),
            instances=Instances.from_dict(d.get('instances', {})),
            build=Build.from_dict(d.get('build', {})),
            run=Run.from_dict(d.get('params', {}), params_class=params_class),
        )

    def panel(self):
        print('rendering groups panel for ' + self.id)
        return pn.Column(
            "**Group: {}**".format(self.id),
            self.param['id'],
            self.instances,
            self.resources,
            self.build,
            self.run.panel(),
        )


class Composition(param.Parameterized):
    metadata = param.Parameter(Metadata(), instantiate=True)
    global_config = param.Parameter(Global(), instantiate=True)

    groups = param.List(precedence=-1)
    group_tabs = pn.Tabs()
    groups_ui = None

    def __init__(self, manifest=None, add_default_group=False, **params):
        super(Composition, self).__init__(**params)
        self.manifest = manifest
        self.testcase_param_classes = dict()
        self._set_manifest(manifest)
        if add_default_group:
            self._add_group()

    @classmethod
    def from_dict(cls, d, manifest=None):
        if manifest is None:
            try:
                manifest = get_manifest(d['global']['plan'])
            except FileNotFoundError:
                print_err("Unable to find manifest for test plan {}. Please import into $TESTGROUND_HOME/plans and try again".format(d['global']['plan']))

        c = Composition(
            manifest=manifest,
            metadata=Metadata.from_dict(d.get('metadata', {})),
            global_config=Global.from_dict(d.get('global', {})),
        )
        params_class = c._params_class_for_current_testcase()
        c.groups = [Group.from_dict(g, params_class=params_class) for g in d.get('groups', [])]

        return c

    @classmethod
    def from_toml_file(cls, filename, manifest=None):
        with open(filename, 'rt') as f:
            d = toml.load(f)
            return cls.from_dict(d, manifest=manifest)

    @param.depends('groups', watch=True)
    def panel(self):
        add_group_button = pn.widgets.Button(name='Add Group')
        add_group_button.on_click(self._add_group)

        self._refresh_tabs()

        if self.groups_ui is None:
            self.groups_ui = pn.Column(
                add_group_button,
                self.group_tabs,
            )

        return pn.Row(
            pn.Column(self.metadata, self.global_config),
            self.groups_ui,
        )

    def _set_manifest(self, manifest):
        if manifest is None:
            return

        g = self.global_config
        print('global conifg: ', g)
        g.set_manifest(manifest)
        for tc in manifest.get('testcases', []):
            self.testcase_param_classes[tc['name']] = make_group_params_class(tc)

    def _params_class_for_current_testcase(self):
        case = self.global_config.case
        cls = self.testcase_param_classes.get(case, None)
        if cls is None:
            print_err("No testcase found in manifest named " + case)
        return cls

    def _add_group(self, *args):
        group_id = 'group-{}'.format(len(self.groups) + 1)
        g = Group(id=group_id, params_class=self._params_class_for_current_testcase())
        g.param.watch(self._refresh_tabs, 'id')
        groups = self.groups
        groups.append(g)
        self.groups = groups
        self.group_tabs.active = len(groups)-1

    @param.depends("global_config.case", watch=True)
    def _test_case_changed(self):
        print('test case changed', self.global_config.case)
        cls = self._params_class_for_current_testcase()
        for g in self.groups:
            g.run.test_params = cls()
        self._refresh_tabs()

    def _refresh_tabs(self, *args):
        self.group_tabs[:] = [(g.id, g.panel()) for g in self.groups]

    def to_dict(self):
        return {
            'metadata': value_dict(self.metadata, renames={'composition_name': 'name'}),
            'global': value_dict(self.global_config),
            'groups': [g.to_dict() for g in self.groups]
        }

    def to_toml(self):
        return toml.dumps(self.to_dict())

    def write_to_file(self, filename):
        with open(filename, 'wt') as f:
            toml.dump(self.to_dict(), f)
