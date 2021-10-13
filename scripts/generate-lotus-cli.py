#!/usr/bin/env python
# Generate lotus command lines documents as text and markdown in folder "lotus/documentation/en".
# Python 2.7

import os


def generate_lotus_cli(prog):
    output_folder = 'documentation/en'
    md_file = open('%s/cli-%s.md' % (output_folder, prog), 'w')  # set the name of md output

    def get_cmd_recursively(cur_cmd):
        depth = cur_cmd.count(' ')
        md_file.writelines(('\n' * min(depth, 1)) + ('#' * depth) + '# ' + cur_cmd[2:] + '\n')

        cmd_flag = False

        print('> ' + cur_cmd)
        cmd_help_output = os.popen(cur_cmd + ' -h')
        cmd_help_output_lines = cmd_help_output.readlines()

        md_file.writelines('```\n')
        md_file.writelines(cmd_help_output_lines)
        md_file.writelines('```\n')

        for line in cmd_help_output_lines:
            try:
                line = line.strip()
                if line == 'COMMANDS:':
                    cmd_flag = True
                if cmd_flag is True and line == '':
                    cmd_flag = False
                if cmd_flag is True and line[-1] != ':' and 'help, h' not in line:
                    gap_pos = 0
                    sub_cmd = line
                    if ' ' in line:
                        gap_pos = sub_cmd.index('  ')
                    if gap_pos:
                        sub_cmd = cur_cmd + ' ' + sub_cmd[:gap_pos]
                    get_cmd_recursively(sub_cmd)
            except Exception as e:
                print('Fail to deal with "%s" with error:\n%s' % (line, e))

    get_cmd_recursively('./' + prog)
    md_file.close()


if __name__ == "__main__":
    # When --help is generated one needs to make sure none of the
    # urfave-cli `EnvVars:` defaults get triggered
    # Unset everything we can find via: grep -ho 'EnvVars:.*' -r * | sort -u
    for e in [ "LOTUS_PATH", "LOTUS_MARKETS_PATH", "LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH", "LOTUS_WORKER_PATH", "WORKER_PATH", "LOTUS_PANIC_REPORT_PATH", "WALLET_PATH" ]:
        os.environ.pop(e, None)

    os.putenv("LOTUS_VERSION_IGNORE_COMMIT", "1")
    generate_lotus_cli('lotus')
    generate_lotus_cli('lotus-miner')
    generate_lotus_cli('lotus-worker')
