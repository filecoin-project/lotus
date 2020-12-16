import toml
import os
import sys


def parse_manifest(manifest_path):
    with open(manifest_path, 'rt') as f:
        return toml.load(f)


def tg_home():
    return os.environ.get('TESTGROUND_HOME',
                          os.path.join(os.environ['HOME'], 'testground'))


def get_plans():
    return list(os.listdir(os.path.join(tg_home(), 'plans')))


def get_manifest(plan_name):
    manifest_path = os.path.join(tg_home(), 'plans', plan_name, 'manifest.toml')
    return parse_manifest(manifest_path)


def print_err(*args):
    print(*args, file=sys.stderr)
