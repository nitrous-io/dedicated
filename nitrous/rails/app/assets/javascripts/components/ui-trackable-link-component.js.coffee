App.UiTrackableLinkComponent = Em.Component.extend
  tagName: 'a'
  attributeBindings: ['href', 'target', 'title']

  text: null
  track: null

  click: (e) ->
    Mp.track @get('track')
    true # bubble / preserve default
