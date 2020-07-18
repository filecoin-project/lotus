#!/usr/bin/env python
# Generate lotus command lines documents as text and markdown in folder "lotus/documentation/en".
# Python 2.7

import os


def generate_lotus_cli():
    output_folder = '../documentation/en'
    txt_file = open('%s/lotus-cli.txt' % output_folder, 'w')  # set the name of txt output
    md_file = open('%s/lotus-cli.md' % output_folder, 'w')  # set the name of md output

    def get_cmd_recursively(cur_cmd):
        txt_file.writelines('\n\n%s\n' % cur_cmd[2:])
        md_file.writelines('#' * cur_cmd.count(' ') + '# ' + cur_cmd[2:] + '\n')

        cmd_flag = False

        cmd_help_output = os.popen('cd ..' + ' && ' + cur_cmd + ' -h')
        cmd_help_output_lines = cmd_help_output.readlines()

        txt_file.writelines(cmd_help_output_lines)
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

    get_cmd_recursively('./lotus')
    txt_file.close()
    md_file.close()


if __name__ == "__main__":
    generate_lotus_cli()
