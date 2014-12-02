App.Router.map ->
  @route 'blank'

  @resource 'boxes', ->
    @route 'new'

  @resource 'box', path: '/boxes/:box_id', ->
    @route 'provisioning'
    @route 'resources'
    @route 'domains'
    @route 'ide'
    @route 'terminal'
    @resource 'box.snapshots', path: '/snapshots', ->
      @route 'new'

  @resource 'public_keys', ->
    @route 'new'

  @resource 'n2o', ->
    @route 'bonus'
    @route 'upgrade'
    @route 'change_plan'

  @resource 'intro', ->
    @route 'create_box'
    @route 'n2o'
    @route 'apps'

  @route 'hack'
  @route 'invites'
  @route 'account'
