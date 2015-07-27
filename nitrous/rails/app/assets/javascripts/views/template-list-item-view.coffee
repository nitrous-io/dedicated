App.TemplateListItemView = Em.View.extend
  templateName: 'template-list-item'
  tagName: 'li'
  classNames: ['template-list-item-view']
  classNameBindings: ['isSelected:selected']

  content: null
  selected: null

  description: Em.computed.alias 'content.description'

  name: (->
    @getWithDefault('content.name', 'Unnamed')
  ).property 'content.name'

  isSelected: (->
    @get('selected') == @get('content')
  ).property 'selected', 'content'

  iconCss: (->
    return unless iconUrl = @get 'content.iconUrl'
    makeBackgroundImageCss(iconUrl)
  ).property 'content.iconUrl'

  defaultIconCss: (->
    makeBackgroundImageCss(App.Template.defaultIconUrl)
  ).property 'App.Template.defaultIconUrl'

  click: ->
    @set('selected', @get('content'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
