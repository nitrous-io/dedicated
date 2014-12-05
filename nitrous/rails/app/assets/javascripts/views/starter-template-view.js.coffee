App.StarterTemplateView = Em.View.extend
  templateName: 'base-image'
  tagName: 'li'
  classNames: ['ui', 'icon-picker']
  classNameBindings: ['isSelected:selected']

  content: null
  selected: null

  name: (->
    @getWithDefault('content.name', 'Empty')
  ).property 'content.name'

  isSelected: (->
    @get('selected') == @get('content.slug')
  ).property 'selected', 'content.slug'

  iconCss: (->
    return unless iconUrl = @get 'content.iconUrl'
    makeBackgroundImageCss(iconUrl)
  ).property 'content.iconUrl'

  defaultIconCss: (->
    makeBackgroundImageCss(App.Template.defaultIconUrl)
  ).property 'App.Template.defaultIconUrl'

  click: ->
    @set('selected', @get('content.slug'))

makeBackgroundImageCss = (url) ->
  return unless url
  "background-image: url(#{url})"
