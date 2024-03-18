module.exports = async ({core, exec}) => {
  const path = require('path');
  const testPaths = []
  await exec.exec('find', ['.', '-name', '*_test.go'], {
    listeners: {
      stdout: (data) => {
        testPaths.push(...data.split('\n'));
      }
    }
  })
  const groups = testPaths.reduce((acc, testPath) => {
    const name = path.basename(testPath, '_test.go');
    const parsedTestPath = path.parse(testPath);
    switch (parsedTestPath.root) {
      case 'itests':
        const group = `itest-${parsedTestPath.name.replace(/_test$/, '')}`;
        acc[group] = {
          paths: [testPath]
        };
        if (['worker', 'deals_concurrent', 'wdpost_worker_config', 'sector_pledge'].includes(name)) {
          acc[group].runner = ['self-hosted', 'linux', 'x64', '2xlarge'];
        }
        if (['wdpost', 'sector_pledge'].includes(name)) {
          acc[group].params = true;
        }
        break;
      case 'node':
        acc['unit-node'].paths.push(testPath);
        break;
      case 'storage':
      case 'extern':
        acc['unit-storage'].paths.push(testPath);
        break;
      case 'cli':
      case 'cmd':
      case 'api':
        acc['unit-cli'].paths.push(testPath);
        break;
      default:
        acc['unit-rest'].paths.push(testPath);
        break;
    }
    return acc;
  }, {
    'multicore-sdr': {
      paths: ['storage/sealer/ffiwrapper/sealer_test.go'],
      flags: '-run TestMulticoreSDR',
      env: {
        GO_TEST_FLAGS: '-run=TestMulticoreSDR',
        TEST_RUSTPROOFS_LOGS: 1
      }
    },
    'conformance': {
      paths: ['conformance/corpus_test.go'],
      env: {
        GO_TEST_FLAGS: '-run=TestConformance',
        SKIP_CONFORMANCE: 0
      },
      params: true,
      statediff: true,
      format: 'pkgname-and-test-fails'
    },
    'unit-node': {
      paths: []
    },
    'unit-storage': {
      paths: [],
      params: true
    },
    'unit-cli': {
      paths: [],
      params: true,
      runner: ['self-hosted', 'linux', 'x64', '2xlarge']
    },
    'unit-rest': {
      paths: [],
      runner: ['self-hosted', 'linux', 'x64', '2xlarge']
    }
  });
  const matrix = Object.entries(groups).map(([name, group]) => {
    return {
      name,
      ...group
    };
  });
  core.info(JSON.stringify(matrix, null, 2));
  core.setOutput('tests', JSON.stringify(matrix));
}
