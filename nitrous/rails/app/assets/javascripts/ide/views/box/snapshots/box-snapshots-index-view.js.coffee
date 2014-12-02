App.BoxSnapshotsIndexView = Em.View.extend
  templateName: 'templates/box/snapshots/index'
  elementId: 'box-snapshots-index-view'
  classNames: ['app_page', 'index', 'settings']

  didClickSnapshot: (event) ->
    @get('controller').newSnapshot()

  didClickDelete: (snapshot) ->
    @get('controller').destroy snapshot

  didClickCancel: (event) ->
    @get('controller').cancel()
