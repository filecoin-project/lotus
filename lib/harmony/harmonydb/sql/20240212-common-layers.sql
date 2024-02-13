INSERT INTO harmony_config (title, config) VALUES
  ('post', '
  [Subsystems]
  EnableWindowPost = true
  Enablewinningpost = true
  '),

  ('gui', '
  [Subsystems]
  EnableWebGui = true
  '),

  ('seal', '
  [Subsystems]
  EnableSealSDR = true
  EnableSealSDRTrees = true
  EnableSendPrecommitMsg = true
  EnablePoRepProof = true
  EnableSendCommitMsg = true
  EnableMoveStorage = true
  ')
  ON CONFLICT (title) DO NOTHING; -- SPs may have these names defined already.