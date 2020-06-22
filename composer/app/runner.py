import os
import panel as pn
import param
from panel.io.server import unlocked
from tornado.ioloop import IOLoop, PeriodicCallback
from tornado.process import Subprocess
from subprocess import STDOUT
from bokeh.models.widgets import Div
from ansi2html import Ansi2HTMLConverter

from .composition import Composition

TESTGROUND = 'testground'


class AnsiColorText(pn.widgets.Widget):
    style = param.Dict(default=None, doc="""
        Dictionary of CSS property:value pairs to apply to this Div.""")

    value = param.Parameter(default=None)

    _format = '<div>{value}</div>'

    _rename = {'name': None, 'value': 'text'}

    # _target_transforms = {'value': 'target.text.split(": ")[0]+": "+value'}
    #
    # _source_transforms = {'value': 'value.split(": ")[1]'}

    _widget_type = Div

    _converter = Ansi2HTMLConverter(inline=True)

    def _process_param_change(self, msg):
        msg = super(AnsiColorText, self)._process_property_change(msg)
        if 'value' in msg:
            text = str(msg.pop('value'))
            text = self._converter.convert(text)
            msg['text'] = text
        return msg

    def scroll_down(self):
        # TODO: figure out how to automatically scroll down as text is added
        pass


class CommandRunner(param.Parameterized):
    command_output = param.String()

    def __init__(self, **params):
        super().__init__(**params)
        self._output_lines = []
        self.proc = None
        self._updater = PeriodicCallback(self._refresh_output, callback_time=1000)

    @pn.depends('command_output')
    def panel(self):
        return pn.Param(self.param, show_name=False, sizing_mode='stretch_width', widgets={
            'command_output': dict(
                type=AnsiColorText,
                sizing_mode='stretch_width',
                height=800)
        })

    def run(self, *cmd):
        self.command_output = ''
        self._output_lines = []
        self.proc = Subprocess(cmd, stdout=Subprocess.STREAM, stderr=STDOUT)
        self._get_next_line()
        self._updater.start()

    def _get_next_line(self):
        if self.proc is None:
            return
        loop = IOLoop.current()
        loop.add_future(self.proc.stdout.read_until(bytes('\n', encoding='utf8')), self._append_output)

    def _append_output(self, future):
        self._output_lines.append(future.result().decode('utf8'))
        self._get_next_line()

    def _refresh_output(self):
        text = ''.join(self._output_lines)
        if len(text) != len(self.command_output):
            with unlocked():
                self.command_output = text


class TestRunner(param.Parameterized):
    composition = param.ClassSelector(class_=Composition, precedence=-1)
    testground_daemon_endpoint = param.String(default="{}:8042".format(os.environ.get('TESTGROUND_DAEMON_HOST', 'localhost')))
    run_test = param.Action(lambda self: self.run())
    runner = CommandRunner()

    def __init__(self, **params):
        super().__init__(**params)

    def run(self):
        # TODO: temp file management - maybe we should mount a volume and save there?
        filename = '/tmp/composition.toml'
        self.composition.write_to_file(filename)

        self.runner.run(TESTGROUND, '--endpoint', self.testground_daemon_endpoint, 'run', 'composition', '-f', filename)

    def panel(self):
        return pn.Column(
            self.param['testground_daemon_endpoint'],
            self.param['run_test'],
            self.runner.panel(),
            sizing_mode='stretch_width',
        )
