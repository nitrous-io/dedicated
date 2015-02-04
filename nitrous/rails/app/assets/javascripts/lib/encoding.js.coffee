#= require encoding
#= require encoding-indexes

utf8Encoder = new window.TextEncoder('utf8')
utf8Decoder = new window.TextDecoder('utf8')

Lib.utf8Encode = (data) =>
  utf8Encoder.encode(data)

Lib.utf8Decode = (data) =>
  utf8Decoder.decode(new DataView(data))
