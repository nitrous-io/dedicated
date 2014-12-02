# colors must be defined in hex values
# must have contrast with editor background (black)
# popover background (white)
# collab pane backbround
App.UserColors =
  colors: -> [
    '#F06060', # red
    '#6DBCEB', # blue
    '#4DA69B', # green
    '#EFC94C', # yellow
    '#E27A3F', # orange
    '#F06060', # pink
    '#6B4B6D', # purple
    '#5C4B51'  # brown
  ]

  nextColor: (->
    currentIndex = 0
    (index) ->
      currentIndex = index if index?
      color = this.colors()[currentIndex % this.colors().length]
      currentIndex = (currentIndex + 1) % this.colors().length
      color
    )()
