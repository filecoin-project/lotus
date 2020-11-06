import param
import panel as pn
import toml
from .util import get_plans, get_manifest
from .composition import Composition
from .runner import TestRunner

STAGE_WELCOME = 'Welcome'
STAGE_CONFIG_COMPOSITION = 'Configure'
STAGE_RUN_TEST = 'Run'


class Welcome(param.Parameterized):
    composition = param.Parameter()
    composition_picker = pn.widgets.FileInput(accept='.toml')
    plan_picker = param.Selector()
    ready = param.Boolean()

    def __init__(self, **params):
        super().__init__(**params)
        self.composition_picker.param.watch(self._composition_updated, 'value')
        self.param.watch(self._plan_selected, 'plan_picker')
        self.param['plan_picker'].objects = ['Select a Plan'] + get_plans()

    def panel(self):
        tabs = pn.Tabs(
            ('New Compostion', self.param['plan_picker']),
            ('Existing Composition', self.composition_picker),
        )

        return pn.Column(
            "Either choose an existing composition or select a plan to create a new composition:",
            tabs,
        )

    def _composition_updated(self, *args):
        print('composition updated')
        content = self.composition_picker.value.decode('utf8')
        comp_toml = toml.loads(content)
        manifest = get_manifest(comp_toml['global']['plan'])
        self.composition = Composition.from_dict(comp_toml, manifest=manifest)
        print('existing composition: {}'.format(self.composition))
        self.ready = True

    def _plan_selected(self, evt):
        if evt.new == 'Select a Plan':
            return
        print('plan selected: {}'.format(evt.new))
        manifest = get_manifest(evt.new)
        self.composition = Composition(manifest=manifest, add_default_group=True)
        print('new composition: ', self.composition)
        self.ready = True


class ConfigureComposition(param.Parameterized):
    composition = param.Parameter()

    @param.depends('composition')
    def panel(self):
        if self.composition is None:
            return pn.Pane("no composition :(")
        print('composition: ', self.composition)
        return self.composition.panel()


class WorkflowPipeline(object):
    def __init__(self):
        stages = [
            (STAGE_WELCOME, Welcome(), dict(ready_parameter='ready')),
            (STAGE_CONFIG_COMPOSITION, ConfigureComposition()),
            (STAGE_RUN_TEST, TestRunner()),
        ]

        self.pipeline = pn.pipeline.Pipeline(debug=True, stages=stages)

    def panel(self):
        return pn.Column(
            pn.Row(
                self.pipeline.title,
                self.pipeline.network,
                self.pipeline.prev_button,
                self.pipeline.next_button,
            ),
            self.pipeline.stage,
            sizing_mode='stretch_width',
        )


class App(object):
    def __init__(self):
        self.workflow = WorkflowPipeline()

    def ui(self):
        return self.workflow.panel().servable("Testground Composer")
