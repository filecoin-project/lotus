const core = require('@actions/core');
const github = require('@actions/github');

async function run() {
    try {
        const token = core.getInput('GITHUB_TOKEN');
        const title = core.getInput('title');
        const label = core.getInput('label');
        const body = core.getInput('body');

        const octokit = github.getOctokit(token);

        const { context } = github;
        const repo = context.repo.repo;
        const owner = context.repo.owner;
        const runId = context.runId;
        const failedActionRun = `https://github.com/${owner}/${repo}/actions/runs/${runId}`;

        const { data: issues } = await octokit.issues.listForRepo({
            owner,
            repo,
            state: 'open',
            labels: label,
        });

        const issueNumber = issues.length > 0 ? issues[0].number : null;

        const bodyWithDetails = `${body} [Action Run link](${failedActionRun})`;

        if (issueNumber) {
            await octokit.issues.createComment({
                owner,
                repo,
                issue_number: issueNumber,
                body: bodyWithDetails,
            });
        } else {
            await octokit.issues.create({
                owner,
                repo,
                title,
                body: bodyWithDetails,
                labels: [label],
            });
        }

        core.setOutput('issue-number', issueNumber);
    } catch (error) {
        core.setFailed(error.message);
    }
}

run();
