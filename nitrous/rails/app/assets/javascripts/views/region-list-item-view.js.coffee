App.RegionListItemView = Em.View.extend
  templateName: 'region-list-item'
  tagName: 'li'
  classNames: ['region-list-item-view']
  classNameBindings: ['isSelected:selected']

  content: null
  selected: null

  isSelected: (->
    @get('selected') == @get('content')
  ).property 'selected', 'content'

  iconCss: (->
    return unless iconUrl = @get 'content.iconUrl'
    makeBackgroundImageCss(iconUrl)
  ).property 'content.iconUrl'

  defaultIconCss: (->
    makeBackgroundImageCss(App.Region.defaultIconUrl)
  ).property 'App.Template.defaultIconUrl'

  click: ->
    @set('selected', @get('content'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
