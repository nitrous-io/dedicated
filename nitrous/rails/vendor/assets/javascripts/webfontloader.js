// edited to always use https
/* Web Font Loader v1.5.6 - (c) Adobe Systems, Google. License: Apache 2.0 */
;(function(window,document,undefined){// Input 0
var COMPILED = !0, goog = goog || {};
goog.global = this;
goog.DEBUG = !1;
goog.LOCALE = "en";
goog.TRUSTED_SITE = !0;
goog.provide = function(a) {
  if(!COMPILED) {
    if(goog.isProvided_(a)) {
      throw Error('Namespace "' + a + '" already declared.');
    }
    delete goog.implicitNamespaces_[a];
    for(var b = a;(b = b.substring(0, b.lastIndexOf("."))) && !goog.getObjectByName(b);) {
      goog.implicitNamespaces_[b] = !0
    }
  }
  goog.exportPath_(a)
};
goog.setTestOnly = function(a) {
  if(COMPILED && !goog.DEBUG) {
    throw a = a || "", Error("Importing test-only code into non-debug environment" + a ? ": " + a : ".");
  }
};
COMPILED || (goog.isProvided_ = function(a) {
  return!goog.implicitNamespaces_[a] && !!goog.getObjectByName(a)
}, goog.implicitNamespaces_ = {});
goog.exportPath_ = function(a, b, c) {
  a = a.split(".");
  c = c || goog.global;
  !(a[0] in c) && c.execScript && c.execScript("var " + a[0]);
  for(var d;a.length && (d = a.shift());) {
    !a.length && goog.isDef(b) ? c[d] = b : c = c[d] ? c[d] : c[d] = {}
  }
};
goog.getObjectByName = function(a, b) {
  for(var c = a.split("."), d = b || goog.global, e;e = c.shift();) {
    if(goog.isDefAndNotNull(d[e])) {
      d = d[e]
    }else {
      return null
    }
  }
  return d
};
goog.globalize = function(a, b) {
  var c = b || goog.global, d;
  for(d in a) {
    c[d] = a[d]
  }
};
goog.addDependency = function(a, b, c) {
  if(!COMPILED) {
    var d;
    a = a.replace(/\\/g, "/");
    for(var e = goog.dependencies_, f = 0;d = b[f];f++) {
      e.nameToPath[d] = a, a in e.pathToNames || (e.pathToNames[a] = {}), e.pathToNames[a][d] = !0
    }
    for(d = 0;b = c[d];d++) {
      a in e.requires || (e.requires[a] = {}), e.requires[a][b] = !0
    }
  }
};
goog.ENABLE_DEBUG_LOADER = !0;
goog.require = function(a) {
  if(!COMPILED && !goog.isProvided_(a)) {
    if(goog.ENABLE_DEBUG_LOADER) {
      var b = goog.getPathFromDeps_(a);
      if(b) {
        goog.included_[b] = !0;
        goog.writeScripts_();
        return
      }
    }
    a = "goog.require could not find: " + a;
    goog.global.console && goog.global.console.error(a);
    throw Error(a);
  }
};
goog.basePath = "";
goog.nullFunction = function() {
};
goog.identityFunction = function(a) {
  return a
};
goog.abstractMethod = function() {
  throw Error("unimplemented abstract method");
};
goog.addSingletonGetter = function(a) {
  a.getInstance = function() {
    if(a.instance_) {
      return a.instance_
    }
    goog.DEBUG && (goog.instantiatedSingletons_[goog.instantiatedSingletons_.length] = a);
    return a.instance_ = new a
  }
};
goog.instantiatedSingletons_ = [];
!COMPILED && goog.ENABLE_DEBUG_LOADER && (goog.included_ = {}, goog.dependencies_ = {pathToNames:{}, nameToPath:{}, requires:{}, visited:{}, written:{}}, goog.inHtmlDocument_ = function() {
  var a = goog.global.document;
  return"undefined" != typeof a && "write" in a
}, goog.findBasePath_ = function() {
  if(goog.global.CLOSURE_BASE_PATH) {
    goog.basePath = goog.global.CLOSURE_BASE_PATH
  }else {
    if(goog.inHtmlDocument_()) {
      for(var a = goog.global.document.getElementsByTagName("script"), b = a.length - 1;0 <= b;--b) {
        var c = a[b].src, d = c.lastIndexOf("?"), d = -1 == d ? c.length : d;
        if("base.js" == c.substr(d - 7, 7)) {
          goog.basePath = c.substr(0, d - 7);
          break
        }
      }
    }
  }
}, goog.importScript_ = function(a) {
  var b = goog.global.CLOSURE_IMPORT_SCRIPT || goog.writeScriptTag_;
  !goog.dependencies_.written[a] && b(a) && (goog.dependencies_.written[a] = !0)
}, goog.writeScriptTag_ = function(a) {
  if(goog.inHtmlDocument_()) {
    var b = goog.global.document;
    if("complete" == b.readyState) {
      if(/\bdeps.js$/.test(a)) {
        return!1
      }
      throw Error('Cannot write "' + a + '" after document load');
    }
    b.write('<script type="text/javascript" src="' + a + '">\x3c/script>');
    return!0
  }
  return!1
}, goog.writeScripts_ = function() {
  function a(e) {
    if(!(e in d.written)) {
      if(!(e in d.visited) && (d.visited[e] = !0, e in d.requires)) {
        for(var g in d.requires[e]) {
          if(!goog.isProvided_(g)) {
            if(g in d.nameToPath) {
              a(d.nameToPath[g])
            }else {
              throw Error("Undefined nameToPath for " + g);
            }
          }
        }
      }
      e in c || (c[e] = !0, b.push(e))
    }
  }
  var b = [], c = {}, d = goog.dependencies_, e;
  for(e in goog.included_) {
    d.written[e] || a(e)
  }
  for(e = 0;e < b.length;e++) {
    if(b[e]) {
      goog.importScript_(goog.basePath + b[e])
    }else {
      throw Error("Undefined script input");
    }
  }
}, goog.getPathFromDeps_ = function(a) {
  return a in goog.dependencies_.nameToPath ? goog.dependencies_.nameToPath[a] : null
}, goog.findBasePath_(), goog.global.CLOSURE_NO_DEPS || goog.importScript_(goog.basePath + "deps.js"));
goog.typeOf = function(a) {
  var b = typeof a;
  if("object" == b) {
    if(a) {
      if(a instanceof Array) {
        return"array"
      }
      if(a instanceof Object) {
        return b
      }
      var c = Object.prototype.toString.call(a);
      if("[object Window]" == c) {
        return"object"
      }
      if("[object Array]" == c || "number" == typeof a.length && "undefined" != typeof a.splice && "undefined" != typeof a.propertyIsEnumerable && !a.propertyIsEnumerable("splice")) {
        return"array"
      }
      if("[object Function]" == c || "undefined" != typeof a.call && "undefined" != typeof a.propertyIsEnumerable && !a.propertyIsEnumerable("call")) {
        return"function"
      }
    }else {
      return"null"
    }
  }else {
    if("function" == b && "undefined" == typeof a.call) {
      return"object"
    }
  }
  return b
};
goog.isDef = function(a) {
  return void 0 !== a
};
goog.isNull = function(a) {
  return null === a
};
goog.isDefAndNotNull = function(a) {
  return null != a
};
goog.isArray = function(a) {
  return"array" == goog.typeOf(a)
};
goog.isArrayLike = function(a) {
  var b = goog.typeOf(a);
  return"array" == b || "object" == b && "number" == typeof a.length
};
goog.isDateLike = function(a) {
  return goog.isObject(a) && "function" == typeof a.getFullYear
};
goog.isString = function(a) {
  return"string" == typeof a
};
goog.isBoolean = function(a) {
  return"boolean" == typeof a
};
goog.isNumber = function(a) {
  return"number" == typeof a
};
goog.isFunction = function(a) {
  return"function" == goog.typeOf(a)
};
goog.isObject = function(a) {
  var b = typeof a;
  return"object" == b && null != a || "function" == b
};
goog.getUid = function(a) {
  return a[goog.UID_PROPERTY_] || (a[goog.UID_PROPERTY_] = ++goog.uidCounter_)
};
goog.removeUid = function(a) {
  "removeAttribute" in a && a.removeAttribute(goog.UID_PROPERTY_);
  try {
    delete a[goog.UID_PROPERTY_]
  }catch(b) {
  }
};
goog.UID_PROPERTY_ = "closure_uid_" + (1E9 * Math.random() >>> 0);
goog.uidCounter_ = 0;
goog.getHashCode = goog.getUid;
goog.removeHashCode = goog.removeUid;
goog.cloneObject = function(a) {
  var b = goog.typeOf(a);
  if("object" == b || "array" == b) {
    if(a.clone) {
      return a.clone()
    }
    var b = "array" == b ? [] : {}, c;
    for(c in a) {
      b[c] = goog.cloneObject(a[c])
    }
    return b
  }
  return a
};
goog.bindNative_ = function(a, b, c) {
  return a.call.apply(a.bind, arguments)
};
goog.bindJs_ = function(a, b, c) {
  if(!a) {
    throw Error();
  }
  if(2 < arguments.length) {
    var d = Array.prototype.slice.call(arguments, 2);
    return function() {
      var c = Array.prototype.slice.call(arguments);
      Array.prototype.unshift.apply(c, d);
      return a.apply(b, c)
    }
  }
  return function() {
    return a.apply(b, arguments)
  }
};
goog.bind = function(a, b, c) {
  goog.bind = Function.prototype.bind && -1 != Function.prototype.bind.toString().indexOf("native code") ? goog.bindNative_ : goog.bindJs_;
  return goog.bind.apply(null, arguments)
};
goog.partial = function(a, b) {
  var c = Array.prototype.slice.call(arguments, 1);
  return function() {
    var b = Array.prototype.slice.call(arguments);
    b.unshift.apply(b, c);
    return a.apply(this, b)
  }
};
goog.mixin = function(a, b) {
  for(var c in b) {
    a[c] = b[c]
  }
};
goog.now = goog.TRUSTED_SITE && Date.now || function() {
  return+new Date
};
goog.globalEval = function(a) {
  if(goog.global.execScript) {
    goog.global.execScript(a, "JavaScript")
  }else {
    if(goog.global.eval) {
      if(null == goog.evalWorksForGlobals_ && (goog.global.eval("var _et_ = 1;"), "undefined" != typeof goog.global._et_ ? (delete goog.global._et_, goog.evalWorksForGlobals_ = !0) : goog.evalWorksForGlobals_ = !1), goog.evalWorksForGlobals_) {
        goog.global.eval(a)
      }else {
        var b = goog.global.document, c = b.createElement("script");
        c.type = "text/javascript";
        c.defer = !1;
        c.appendChild(b.createTextNode(a));
        b.body.appendChild(c);
        b.body.removeChild(c)
      }
    }else {
      throw Error("goog.globalEval not available");
    }
  }
};
goog.evalWorksForGlobals_ = null;
goog.getCssName = function(a, b) {
  var c = function(a) {
    return goog.cssNameMapping_[a] || a
  }, d = function(a) {
    a = a.split("-");
    for(var b = [], d = 0;d < a.length;d++) {
      b.push(c(a[d]))
    }
    return b.join("-")
  }, d = goog.cssNameMapping_ ? "BY_WHOLE" == goog.cssNameMappingStyle_ ? c : d : function(a) {
    return a
  };
  return b ? a + "-" + d(b) : d(a)
};
goog.setCssNameMapping = function(a, b) {
  goog.cssNameMapping_ = a;
  goog.cssNameMappingStyle_ = b
};
!COMPILED && goog.global.CLOSURE_CSS_NAME_MAPPING && (goog.cssNameMapping_ = goog.global.CLOSURE_CSS_NAME_MAPPING);
goog.getMsg = function(a, b) {
  var c = b || {}, d;
  for(d in c) {
    var e = ("" + c[d]).replace(/\$/g, "$$$$");
    a = a.replace(RegExp("\\{\\$" + d + "\\}", "gi"), e)
  }
  return a
};
goog.getMsgWithFallback = function(a) {
  return a
};
goog.exportSymbol = function(a, b, c) {
  goog.exportPath_(a, b, c)
};
goog.exportProperty = function(a, b, c) {
  a[b] = c
};
goog.inherits = function(a, b) {
  function c() {
  }
  c.prototype = b.prototype;
  a.superClass_ = b.prototype;
  a.prototype = new c;
  a.prototype.constructor = a
};
goog.base = function(a, b, c) {
  var d = arguments.callee.caller;
  if(d.superClass_) {
    return d.superClass_.constructor.apply(a, Array.prototype.slice.call(arguments, 1))
  }
  for(var e = Array.prototype.slice.call(arguments, 2), f = !1, g = a.constructor;g;g = g.superClass_ && g.superClass_.constructor) {
    if(g.prototype[b] === d) {
      f = !0
    }else {
      if(f) {
        return g.prototype[b].apply(a, e)
      }
    }
  }
  if(a[b] === d) {
    return a.constructor.prototype[b].apply(a, e)
  }
  throw Error("goog.base called from a method of one name to a method of a different name");
};
goog.scope = function(a) {
  a.call(goog.global)
};
// Input 1
var webfont = {DomHelper:function(a, b) {
  this.mainWindow_ = a;
  this.loadWindow_ = b || a;
  this.document_ = this.loadWindow_.document
}};
webfont.DomHelper.prototype.createElement = function(a, b, c) {
  a = this.document_.createElement(a);
  if(b) {
    for(var d in b) {
      b.hasOwnProperty(d) && ("style" == d ? this.setStyle(a, b[d]) : a.setAttribute(d, b[d]))
    }
  }
  c && a.appendChild(this.document_.createTextNode(c));
  return a
};
webfont.DomHelper.prototype.insertInto = function(a, b) {
  var c = this.document_.getElementsByTagName(a)[0];
  c || (c = document.documentElement);
  return c && c.lastChild ? (c.insertBefore(b, c.lastChild), !0) : !1
};
webfont.DomHelper.prototype.whenBodyExists = function(a) {
  var b = this, c = function() {
    b.document_.body ? a() : setTimeout(c, 0)
  };
  c()
};
webfont.DomHelper.prototype.removeElement = function(a) {
  return a.parentNode ? (a.parentNode.removeChild(a), !0) : !1
};
webfont.DomHelper.prototype.appendClassName = function(a, b) {
  this.updateClassName(a, [b])
};
webfont.DomHelper.prototype.removeClassName = function(a, b) {
  this.updateClassName(a, null, [b])
};
webfont.DomHelper.prototype.updateClassName = function(a, b, c) {
  b = b || [];
  c = c || [];
  for(var d = a.className.split(/\s+/), e = 0;e < b.length;e += 1) {
    for(var f = !1, g = 0;g < d.length;g += 1) {
      if(b[e] === d[g]) {
        f = !0;
        break
      }
    }
    f || d.push(b[e])
  }
  b = [];
  for(e = 0;e < d.length;e += 1) {
    f = !1;
    for(g = 0;g < c.length;g += 1) {
      if(d[e] === c[g]) {
        f = !0;
        break
      }
    }
    f || b.push(d[e])
  }
  a.className = b.join(" ").replace(/\s+/g, " ").replace(/^\s+|\s+$/, "")
};
webfont.DomHelper.prototype.hasClassName = function(a, b) {
  for(var c = a.className.split(/\s+/), d = 0, e = c.length;d < e;d++) {
    if(c[d] == b) {
      return!0
    }
  }
  return!1
};
webfont.DomHelper.prototype.setStyle = function(a, b) {
  a.style.cssText = b
};
webfont.DomHelper.prototype.getMainWindow = function() {
  return this.mainWindow_
};
webfont.DomHelper.prototype.getLoadWindow = function() {
  return this.loadWindow_
};
webfont.DomHelper.prototype.getProtocol = function() {
  //var a = this.loadWindow_.location.protocol;
  //"about:" == a && (a = this.mainWindow_.location.protocol);
  //return"https:" == a ? "https:" : "http:"
  return "https:"
};
webfont.DomHelper.prototype.isHttps = function() {
  return"https:" === this.getProtocol()
};
webfont.DomHelper.prototype.getHostName = function() {
  return this.getLoadWindow().location.hostname || this.getMainWindow().location.hostname
};
webfont.DomHelper.prototype.createStyle = function(a) {
  var b = this.createElement("style");
  b.setAttribute("type", "text/css");
  b.styleSheet ? b.styleSheet.cssText = a : b.appendChild(document.createTextNode(a));
  return b
};
webfont.DomHelper.prototype.loadStylesheet = function(a, b) {
  var c = this.createElement("link", {rel:"stylesheet", href:a}), d = !1;
  c.onload = function() {
    d || (d = !0, b && b(null))
  };
  c.onerror = function() {
    d || (d = !0, b && b(Error("Stylesheet failed to load")))
  };
  this.insertInto("head", c);
  return c
};
webfont.DomHelper.prototype.loadScript = function(a, b, c) {
  var d = this.document_.getElementsByTagName("head")[0];
  if(d) {
    var e = this.createElement("script", {src:a}), f = !1;
    e.onload = e.onreadystatechange = function() {
      if(!f && (!this.readyState || "loaded" == this.readyState || "complete" == this.readyState)) {
        f = !0, b && b(null), e.onload = e.onreadystatechange = null, "HEAD" == e.parentNode.tagName && d.removeChild(e)
      }
    };
    d.appendChild(e);
    window.setTimeout(function() {
      f || (f = !0, b && b(Error("Script load timeout")))
    }, c || 5E3);
    return e
  }
  return null
};
// Input 2
webfont.BrowserInfo = function(a, b, c) {
  this.webfontSupport_ = a;
  this.webKitFallbackBug_ = b;
  this.webKitMetricsBug_ = c
};
goog.exportSymbol("webfont.BrowserInfo", webfont.BrowserInfo);
webfont.BrowserInfo.prototype.hasWebFontSupport = function() {
  return this.webfontSupport_
};
goog.exportProperty(webfont.BrowserInfo.prototype, "hasWebFontSupport", webfont.BrowserInfo.prototype.hasWebFontSupport);
webfont.BrowserInfo.prototype.hasWebKitFallbackBug = function() {
  return this.webKitFallbackBug_
};
goog.exportProperty(webfont.BrowserInfo.prototype, "hasWebKitFallbackBug", webfont.BrowserInfo.prototype.hasWebKitFallbackBug);
webfont.BrowserInfo.prototype.hasWebKitMetricsBug = function() {
  return this.webKitMetricsBug_
};
goog.exportProperty(webfont.BrowserInfo.prototype, "hasWebKitMetricsBug", webfont.BrowserInfo.prototype.hasWebKitMetricsBug);
// Input 3
webfont.Version = function(a, b, c, d) {
  this.major = goog.isDefAndNotNull(a) ? a : null;
  this.minor = goog.isDefAndNotNull(b) ? b : null;
  this.patch = goog.isDefAndNotNull(c) ? c : null;
  this.build = goog.isDefAndNotNull(d) ? d : null
};
webfont.Version.TOKENIZER = /^([0-9]+)(?:[\._-]([0-9]+))?(?:[\._-]([0-9]+))?(?:[\._+-]?(.*))?$/;
webfont.Version.prototype.isValid = function() {
  return!goog.isNull(this.major)
};
webfont.Version.prototype.compare = function(a) {
  return this.major > a.major || this.major === a.major && this.minor > a.minor || this.major === a.major && this.minor === a.minor && this.patch > a.patch ? 1 : this.major < a.major || this.major === a.major && this.minor < a.minor || this.major === a.major && this.minor === a.minor && this.patch < a.patch ? -1 : 0
};
webfont.Version.prototype.gt = function(a) {
  return 1 === this.compare(a)
};
webfont.Version.prototype.lt = function(a) {
  return-1 === this.compare(a)
};
webfont.Version.prototype.ge = function(a) {
  return 0 === this.compare(a) || 1 === this.compare(a)
};
webfont.Version.prototype.le = function(a) {
  return 0 === this.compare(a) || -1 === this.compare(a)
};
webfont.Version.prototype.eq = function(a) {
  return 0 === this.compare(a)
};
webfont.Version.prototype.ne = function(a) {
  return 0 !== this.compare(a)
};
webfont.Version.prototype.toString = function() {
  return[this.major, this.minor || "", this.patch || "", this.build || ""].join("")
};
webfont.Version.parse = function(a) {
  a = webfont.Version.TOKENIZER.exec(a);
  var b = null, c = null, d = null, e = null;
  a && (!goog.isNull(a[1]) && a[1] && (b = parseInt(a[1], 10)), !goog.isNull(a[2]) && a[2] && (c = parseInt(a[2], 10)), !goog.isNull(a[3]) && a[3] && (d = parseInt(a[3], 10)), !goog.isNull(a[4]) && a[4] && (e = /^[0-9]+$/.test(a[4]) ? parseInt(a[4], 10) : a[4]));
  return new webfont.Version(b, c, d, e)
};
// Input 4
webfont.UserAgent = function(a, b, c, d, e, f, g, h, j, k, m) {
  this.name_ = a;
  this.version_ = b;
  this.versionString_ = c;
  this.engine_ = d;
  this.engineVersion_ = e;
  this.engineVersionString_ = f;
  this.platform_ = g;
  this.platformVersion_ = h;
  this.platformVersionString_ = j;
  this.documentMode_ = k;
  this.browserInfo_ = m
};
goog.exportSymbol("webfont.UserAgent", webfont.UserAgent);
webfont.UserAgent.prototype.getName = function() {
  return this.name_
};
goog.exportProperty(webfont.UserAgent.prototype, "getName", webfont.UserAgent.prototype.getName);
webfont.UserAgent.prototype.getVersion = function() {
  return this.versionString_
};
goog.exportProperty(webfont.UserAgent.prototype, "getVersion", webfont.UserAgent.prototype.getVersion);
webfont.UserAgent.prototype.getParsedVersion = function() {
  return this.version_
};
webfont.UserAgent.prototype.getEngine = function() {
  return this.engine_
};
goog.exportProperty(webfont.UserAgent.prototype, "getEngine", webfont.UserAgent.prototype.getEngine);
webfont.UserAgent.prototype.getEngineVersion = function() {
  return this.engineVersionString_
};
goog.exportProperty(webfont.UserAgent.prototype, "getEngineVersion", webfont.UserAgent.prototype.getEngineVersion);
webfont.UserAgent.prototype.getParsedEngineVersion = function() {
  return this.engineVersion_
};
webfont.UserAgent.prototype.getPlatform = function() {
  return this.platform_
};
goog.exportProperty(webfont.UserAgent.prototype, "getPlatform", webfont.UserAgent.prototype.getPlatform);
webfont.UserAgent.prototype.getPlatformVersion = function() {
  return this.platformVersionString_
};
goog.exportProperty(webfont.UserAgent.prototype, "getPlatformVersion", webfont.UserAgent.prototype.getPlatformVersion);
webfont.UserAgent.prototype.getParsedPlatformVersion = function() {
  return this.platformVersion_
};
webfont.UserAgent.prototype.getDocumentMode = function() {
  return this.documentMode_
};
goog.exportProperty(webfont.UserAgent.prototype, "getDocumentMode", webfont.UserAgent.prototype.getDocumentMode);
webfont.UserAgent.prototype.getBrowserInfo = function() {
  return this.browserInfo_
};
goog.exportProperty(webfont.UserAgent.prototype, "getBrowserInfo", webfont.UserAgent.prototype.getBrowserInfo);
// Input 5
webfont.UserAgentParser = function(a, b) {
  this.userAgent_ = a;
  this.doc_ = b
};
webfont.UserAgentParser.UNKNOWN = "Unknown";
webfont.UserAgentParser.BUILTIN_BROWSER = "BuiltinBrowser";
webfont.UserAgentParser.UNKNOWN_USER_AGENT = new webfont.UserAgent(webfont.UserAgentParser.UNKNOWN, new webfont.Version, webfont.UserAgentParser.UNKNOWN, webfont.UserAgentParser.UNKNOWN, new webfont.Version, webfont.UserAgentParser.UNKNOWN, webfont.UserAgentParser.UNKNOWN, new webfont.Version, webfont.UserAgentParser.UNKNOWN, void 0, new webfont.BrowserInfo(!1, !1, !1));
webfont.UserAgentParser.prototype.parse = function() {
  return this.isIe_() ? this.parseIeUserAgentString_() : this.isOldOpera_() ? this.parseOldOperaUserAgentString_() : this.isOpera_() ? this.parseWebKitUserAgentString_() : this.isWebKit_() ? this.parseWebKitUserAgentString_() : this.isGecko_() ? this.parseGeckoUserAgentString_() : webfont.UserAgentParser.UNKNOWN_USER_AGENT
};
webfont.UserAgentParser.prototype.getPlatform_ = function() {
  var a = this.getMatchingGroup_(this.userAgent_, /(iPod|iPad|iPhone|Android|Windows Phone|BB\d{2}|BlackBerry)/, 1);
  if("" != a) {
    return/BB\d{2}/.test(a) && (a = "BlackBerry"), a
  }
  a = this.getMatchingGroup_(this.userAgent_, /(Linux|Mac_PowerPC|Macintosh|Windows|CrOS|PlayStation|CrKey)/, 1);
  return"" != a ? ("Mac_PowerPC" == a ? a = "Macintosh" : "PlayStation" == a && (a = "Linux"), a) : webfont.UserAgentParser.UNKNOWN
};
webfont.UserAgentParser.prototype.getPlatformVersionString_ = function() {
  var a = this.getMatchingGroup_(this.userAgent_, /(OS X|Windows NT|Android) ([^;)]+)/, 2);
  if(a || (a = this.getMatchingGroup_(this.userAgent_, /Windows Phone( OS)? ([^;)]+)/, 2)) || (a = this.getMatchingGroup_(this.userAgent_, /(iPhone )?OS ([\d_]+)/, 2))) {
    return a
  }
  if(a = this.getMatchingGroup_(this.userAgent_, /(?:Linux|CrOS|CrKey) ([^;)]+)/, 1)) {
    for(var a = a.split(/\s/), b = 0;b < a.length;b += 1) {
      if(/^[\d\._]+$/.test(a[b])) {
        return a[b]
      }
    }
  }
  return(a = this.getMatchingGroup_(this.userAgent_, /(BB\d{2}|BlackBerry).*?Version\/([^\s]*)/, 2)) ? a : webfont.UserAgentParser.UNKNOWN
};
webfont.UserAgentParser.prototype.isIe_ = function() {
  return-1 != this.userAgent_.indexOf("MSIE") || -1 != this.userAgent_.indexOf("Trident/")
};
webfont.UserAgentParser.prototype.parseIeUserAgentString_ = function() {
  var a = this.getPlatform_(), b = this.getPlatformVersionString_(), c = webfont.Version.parse(b), d = null, e = null, f = null, g = null, h = this.getMatchingGroup_(this.userAgent_, /Trident\/([\d\w\.]+)/, 1), j = this.getDocumentMode_(this.doc_), d = -1 != this.userAgent_.indexOf("MSIE") ? this.getMatchingGroup_(this.userAgent_, /MSIE ([\d\w\.]+)/, 1) : this.getMatchingGroup_(this.userAgent_, /rv:([\d\w\.]+)/, 1), e = webfont.Version.parse(d);
  "" != h ? (f = "Trident", g = webfont.Version.parse(h)) : (f = webfont.UserAgentParser.UNKNOWN, g = new webfont.Version, h = webfont.UserAgentParser.UNKNOWN);
  return new webfont.UserAgent("MSIE", e, d, f, g, h, a, c, b, j, new webfont.BrowserInfo("Windows" == a && 6 <= e.major || "Windows Phone" == a && 8 <= c.major, !1, !1))
};
webfont.UserAgentParser.prototype.isOldOpera_ = function() {
  return-1 != this.userAgent_.indexOf("Opera")
};
webfont.UserAgentParser.prototype.isOpera_ = function() {
  return/OPR\/[\d.]+/.test(this.userAgent_)
};
webfont.UserAgentParser.prototype.parseOldOperaUserAgentString_ = function() {
  var a = webfont.UserAgentParser.UNKNOWN, b = this.getMatchingGroup_(this.userAgent_, /Presto\/([\d\w\.]+)/, 1), c = webfont.Version.parse(b), d = this.getPlatformVersionString_(), e = webfont.Version.parse(d), f = this.getDocumentMode_(this.doc_);
  c.isValid() ? a = "Presto" : (-1 != this.userAgent_.indexOf("Gecko") && (a = "Gecko"), b = this.getMatchingGroup_(this.userAgent_, /rv:([^\)]+)/, 1), c = webfont.Version.parse(b));
  if(-1 != this.userAgent_.indexOf("Opera Mini/")) {
    var g = this.getMatchingGroup_(this.userAgent_, /Opera Mini\/([\d\.]+)/, 1), h = webfont.Version.parse(g);
    return new webfont.UserAgent("OperaMini", h, g, a, c, b, this.getPlatform_(), e, d, f, new webfont.BrowserInfo(!1, !1, !1))
  }
  if(-1 != this.userAgent_.indexOf("Version/") && (g = this.getMatchingGroup_(this.userAgent_, /Version\/([\d\.]+)/, 1), h = webfont.Version.parse(g), h.isValid())) {
    return new webfont.UserAgent("Opera", h, g, a, c, b, this.getPlatform_(), e, d, f, new webfont.BrowserInfo(10 <= h.major, !1, !1))
  }
  g = this.getMatchingGroup_(this.userAgent_, /Opera[\/ ]([\d\.]+)/, 1);
  h = webfont.Version.parse(g);
  return h.isValid() ? new webfont.UserAgent("Opera", h, g, a, c, b, this.getPlatform_(), e, d, f, new webfont.BrowserInfo(10 <= h.major, !1, !1)) : new webfont.UserAgent("Opera", new webfont.Version, webfont.UserAgentParser.UNKNOWN, a, c, b, this.getPlatform_(), e, d, f, new webfont.BrowserInfo(!1, !1, !1))
};
webfont.UserAgentParser.prototype.isWebKit_ = function() {
  return/AppleWeb(K|k)it/.test(this.userAgent_)
};
webfont.UserAgentParser.prototype.parseWebKitUserAgentString_ = function() {
  var a = this.getPlatform_(), b = this.getPlatformVersionString_(), c = webfont.Version.parse(b), d = this.getMatchingGroup_(this.userAgent_, /AppleWeb(?:K|k)it\/([\d\.\+]+)/, 1), e = webfont.Version.parse(d), f = webfont.UserAgentParser.UNKNOWN, g = new webfont.Version, h = webfont.UserAgentParser.UNKNOWN, j = !1;
  /OPR\/[\d.]+/.test(this.userAgent_) ? f = "Opera" : -1 != this.userAgent_.indexOf("Chrome") || -1 != this.userAgent_.indexOf("CrMo") || -1 != this.userAgent_.indexOf("CriOS") ? f = "Chrome" : /Silk\/\d/.test(this.userAgent_) ? f = "Silk" : "BlackBerry" == a || "Android" == a ? f = webfont.UserAgentParser.BUILTIN_BROWSER : -1 != this.userAgent_.indexOf("PhantomJS") ? f = "PhantomJS" : -1 != this.userAgent_.indexOf("Safari") ? f = "Safari" : -1 != this.userAgent_.indexOf("AdobeAIR") ? f = "AdobeAIR" :
  -1 != this.userAgent_.indexOf("PlayStation") && (f = "BuiltinBrowser");
  f == webfont.UserAgentParser.BUILTIN_BROWSER ? h = webfont.UserAgentParser.UNKNOWN : "Silk" == f ? h = this.getMatchingGroup_(this.userAgent_, /Silk\/([\d\._]+)/, 1) : "Chrome" == f ? h = this.getMatchingGroup_(this.userAgent_, /(Chrome|CrMo|CriOS)\/([\d\.]+)/, 2) : -1 != this.userAgent_.indexOf("Version/") ? h = this.getMatchingGroup_(this.userAgent_, /Version\/([\d\.\w]+)/, 1) : "AdobeAIR" == f ? h = this.getMatchingGroup_(this.userAgent_, /AdobeAIR\/([\d\.]+)/, 1) : "Opera" == f ? h = this.getMatchingGroup_(this.userAgent_,
  /OPR\/([\d.]+)/, 1) : "PhantomJS" == f && (h = this.getMatchingGroup_(this.userAgent_, /PhantomJS\/([\d.]+)/, 1));
  var g = webfont.Version.parse(h), j = "AdobeAIR" == f ? 2 < g.major || 2 == g.major && 5 <= g.minor : "BlackBerry" == a ? 10 <= c.major : "Android" == a ? 2 < c.major || 2 == c.major && 1 < c.minor : 526 <= e.major || 525 <= e.major && 13 <= e.minor, k = 536 > e.major || 536 == e.major && 11 > e.minor, m = "iPhone" == a || "iPad" == a || "iPod" == a || "Macintosh" == a;
  return new webfont.UserAgent(f, g, h, "AppleWebKit", e, d, a, c, b, this.getDocumentMode_(this.doc_), new webfont.BrowserInfo(j, k, m))
};
webfont.UserAgentParser.prototype.isGecko_ = function() {
  return-1 != this.userAgent_.indexOf("Gecko")
};
webfont.UserAgentParser.prototype.parseGeckoUserAgentString_ = function() {
  var a = webfont.UserAgentParser.UNKNOWN, b = new webfont.Version, c = webfont.UserAgentParser.UNKNOWN, d = this.getPlatformVersionString_(), e = webfont.Version.parse(d), f = !1;
  -1 != this.userAgent_.indexOf("Firefox") ? (a = "Firefox", c = this.getMatchingGroup_(this.userAgent_, /Firefox\/([\d\w\.]+)/, 1), b = webfont.Version.parse(c), f = 3 <= b.major && 5 <= b.minor) : -1 != this.userAgent_.indexOf("Mozilla") && (a = "Mozilla");
  var g = this.getMatchingGroup_(this.userAgent_, /rv:([^\)]+)/, 1), h = webfont.Version.parse(g);
  f || (f = 1 < h.major || 1 == h.major && 9 < h.minor || 1 == h.major && 9 == h.minor && 2 <= h.patch || null != g.match(/1\.9\.1b[123]/) || null != g.match(/1\.9\.1\.[\d\.]+/));
  return new webfont.UserAgent(a, b, c, "Gecko", h, g, this.getPlatform_(), e, d, this.getDocumentMode_(this.doc_), new webfont.BrowserInfo(f, !1, !1))
};
webfont.UserAgentParser.prototype.getMatchingGroup_ = function(a, b, c) {
  return(a = a.match(b)) && a[c] ? a[c] : ""
};
webfont.UserAgentParser.prototype.getDocumentMode_ = function(a) {
  if(a.documentMode) {
    return a.documentMode
  }
};
// Input 6
webfont.CssClassName = function(a) {
  this.joinChar_ = a || webfont.CssClassName.DEFAULT_JOIN_CHAR
};
webfont.CssClassName.DEFAULT_JOIN_CHAR = "-";
webfont.CssClassName.prototype.sanitize = function(a) {
  return a.replace(/[\W_]+/g, "").toLowerCase()
};
webfont.CssClassName.prototype.build = function(a) {
  for(var b = [], c = 0;c < arguments.length;c++) {
    b.push(this.sanitize(arguments[c]))
  }
  return b.join(this.joinChar_)
};
// Input 7
webfont.Font = function(a, b) {
  this.name_ = a;
  this.weight_ = 4;
  this.style_ = "n";
  var c = (b || "n4").match(/^([nio])([1-9])$/i);
  c && (this.style_ = c[1], this.weight_ = parseInt(c[2], 10))
};
webfont.Font.prototype.getName = function() {
  return this.name_
};
webfont.Font.prototype.getCssName = function() {
  return this.quote_(this.name_)
};
webfont.Font.prototype.quote_ = function(a) {
  var b = [];
  a = a.split(/,\s*/);
  for(var c = 0;c < a.length;c++) {
    var d = a[c].replace(/['"]/g, "");
    -1 == d.indexOf(" ") ? b.push(d) : b.push("'" + d + "'")
  }
  return b.join(",")
};
webfont.Font.prototype.getVariation = function() {
  return this.style_ + this.weight_
};
webfont.Font.prototype.getCssVariation = function() {
  var a = "normal", b = this.weight_ + "00";
  "o" === this.style_ ? a = "oblique" : "i" === this.style_ && (a = "italic");
  return"font-style:" + a + ";font-weight:" + b + ";"
};
webfont.Font.parseCssVariation = function(a) {
  var b = 4, c = "n", d = null;
  a && ((d = a.match(/(normal|oblique|italic)/i)) && d[1] && (c = d[1].substr(0, 1).toLowerCase()), (d = a.match(/([1-9]00|normal|bold)/i)) && d[1] && (/bold/i.test(d[1]) ? b = 7 : /[1-9]00/.test(d[1]) && (b = parseInt(d[1].substr(0, 1), 10))));
  return c + b
};
// Input 8
webfont.EventDispatcher = function(a, b, c, d) {
  this.domHelper_ = a;
  this.htmlElement_ = b;
  this.callbacks_ = c;
  this.namespace_ = d || webfont.EventDispatcher.DEFAULT_NAMESPACE;
  this.cssClassName_ = new webfont.CssClassName("-")
};
webfont.EventDispatcher.DEFAULT_NAMESPACE = "wf";
webfont.EventDispatcher.LOADING = "loading";
webfont.EventDispatcher.ACTIVE = "active";
webfont.EventDispatcher.INACTIVE = "inactive";
webfont.EventDispatcher.FONT = "font";
webfont.EventDispatcher.prototype.dispatchLoading = function() {
  this.domHelper_.updateClassName(this.htmlElement_, [this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.LOADING)]);
  this.dispatch_(webfont.EventDispatcher.LOADING)
};
webfont.EventDispatcher.prototype.dispatchFontLoading = function(a) {
  this.domHelper_.updateClassName(this.htmlElement_, [this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.LOADING)]);
  this.dispatch_(webfont.EventDispatcher.FONT + webfont.EventDispatcher.LOADING, a)
};
webfont.EventDispatcher.prototype.dispatchFontActive = function(a) {
  this.domHelper_.updateClassName(this.htmlElement_, [this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.ACTIVE)], [this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.LOADING), this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.INACTIVE)]);
  this.dispatch_(webfont.EventDispatcher.FONT + webfont.EventDispatcher.ACTIVE, a)
};
webfont.EventDispatcher.prototype.dispatchFontInactive = function(a) {
  var b = this.domHelper_.hasClassName(this.htmlElement_, this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.ACTIVE)), c = [], d = [this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.LOADING)];
  b || c.push(this.cssClassName_.build(this.namespace_, a.getName(), a.getVariation().toString(), webfont.EventDispatcher.INACTIVE));
  this.domHelper_.updateClassName(this.htmlElement_, c, d);
  this.dispatch_(webfont.EventDispatcher.FONT + webfont.EventDispatcher.INACTIVE, a)
};
webfont.EventDispatcher.prototype.dispatchInactive = function() {
  var a = this.domHelper_.hasClassName(this.htmlElement_, this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.ACTIVE)), b = [], c = [this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.LOADING)];
  a || b.push(this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.INACTIVE));
  this.domHelper_.updateClassName(this.htmlElement_, b, c);
  this.dispatch_(webfont.EventDispatcher.INACTIVE)
};
webfont.EventDispatcher.prototype.dispatchActive = function() {
  this.domHelper_.updateClassName(this.htmlElement_, [this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.ACTIVE)], [this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.LOADING), this.cssClassName_.build(this.namespace_, webfont.EventDispatcher.INACTIVE)]);
  this.dispatch_(webfont.EventDispatcher.ACTIVE)
};
webfont.EventDispatcher.prototype.dispatch_ = function(a, b) {
  if(this.callbacks_[a]) {
    if(b) {
      this.callbacks_[a](b.getName(), b.getVariation())
    }else {
      this.callbacks_[a]()
    }
  }
};
// Input 9
webfont.FontModule = function() {
};
webfont.FontModule.prototype.supportUserAgent = function() {
};
webfont.FontModule.prototype.load = function() {
};
// Input 10
webfont.FontModuleLoader = function() {
  this.modules_ = {}
};
webfont.FontModuleLoader.prototype.addModuleFactory = function(a, b) {
  this.modules_[a] = b
};
webfont.FontModuleLoader.prototype.getModules = function(a, b) {
  var c = [], d;
  for(d in a) {
    if(a.hasOwnProperty(d)) {
      var e = this.modules_[d];
      e && c.push(e(a[d], b))
    }
  }
  return c
};
// Input 11
webfont.FontRuler = function(a, b) {
  this.domHelper_ = a;
  this.fontTestString_ = b;
  this.el_ = this.domHelper_.createElement("span", {"aria-hidden":"true"}, this.fontTestString_)
};
webfont.FontRuler.prototype.setFont = function(a) {
  this.domHelper_.setStyle(this.el_, this.computeStyleString_(a))
};
webfont.FontRuler.prototype.insert = function() {
  this.domHelper_.insertInto("body", this.el_)
};
webfont.FontRuler.prototype.computeStyleString_ = function(a) {
  return"display:block;position:absolute;top:-999px;left:-999px;font-size:300px;width:auto;height:auto;line-height:normal;margin:0;padding:0;font-variant:normal;white-space:nowrap;font-family:" + a.getCssName() + ";" + a.getCssVariation()
};
webfont.FontRuler.prototype.getWidth = function() {
  return this.el_.offsetWidth
};
webfont.FontRuler.prototype.remove = function() {
  this.domHelper_.removeElement(this.el_)
};
// Input 12
webfont.FontWatchRunner = function(a, b, c, d, e, f, g, h) {
  this.activeCallback_ = a;
  this.inactiveCallback_ = b;
  this.domHelper_ = c;
  this.font_ = d;
  this.fontTestString_ = h || webfont.FontWatchRunner.DEFAULT_TEST_STRING;
  this.browserInfo_ = e;
  this.lastResortWidths_ = {};
  this.timeout_ = f || 5E3;
  this.metricCompatibleFonts_ = g || null;
  this.fontRulerB_ = this.fontRulerA_ = null;
  this.setupLastResortWidths_()
};
webfont.FontWatchRunner.LastResortFonts = {SERIF:"serif", SANS_SERIF:"sans-serif", MONOSPACE:"monospace"};
webfont.FontWatchRunner.DEFAULT_TEST_STRING = "BESbswy";
webfont.FontWatchRunner.prototype.setupLastResortWidths_ = function() {
  var a = new webfont.FontRuler(this.domHelper_, this.fontTestString_);
  a.insert();
  for(var b in webfont.FontWatchRunner.LastResortFonts) {
    webfont.FontWatchRunner.LastResortFonts.hasOwnProperty(b) && (a.setFont(new webfont.Font(webfont.FontWatchRunner.LastResortFonts[b], this.font_.getVariation())), this.lastResortWidths_[webfont.FontWatchRunner.LastResortFonts[b]] = a.getWidth())
  }
  a.remove()
};
webfont.FontWatchRunner.prototype.start = function() {
  this.fontRulerA_ = new webfont.FontRuler(this.domHelper_, this.fontTestString_);
  this.fontRulerA_.insert();
  this.fontRulerB_ = new webfont.FontRuler(this.domHelper_, this.fontTestString_);
  this.fontRulerB_.insert();
  this.started_ = goog.now();
  this.fontRulerA_.setFont(new webfont.Font(this.font_.getName() + "," + webfont.FontWatchRunner.LastResortFonts.SERIF, this.font_.getVariation()));
  this.fontRulerB_.setFont(new webfont.Font(this.font_.getName() + "," + webfont.FontWatchRunner.LastResortFonts.SANS_SERIF, this.font_.getVariation()));
  this.check_()
};
webfont.FontWatchRunner.prototype.widthMatches_ = function(a, b) {
  return a === this.lastResortWidths_[b]
};
webfont.FontWatchRunner.prototype.widthsMatchLastResortWidths_ = function(a, b) {
  for(var c in webfont.FontWatchRunner.LastResortFonts) {
    if(webfont.FontWatchRunner.LastResortFonts.hasOwnProperty(c) && this.widthMatches_(a, webfont.FontWatchRunner.LastResortFonts[c]) && this.widthMatches_(b, webfont.FontWatchRunner.LastResortFonts[c])) {
      return!0
    }
  }
  return!1
};
webfont.FontWatchRunner.prototype.hasTimedOut_ = function() {
  return goog.now() - this.started_ >= this.timeout_
};
webfont.FontWatchRunner.prototype.isFallbackFont_ = function(a, b) {
  return this.widthMatches_(a, webfont.FontWatchRunner.LastResortFonts.SERIF) && this.widthMatches_(b, webfont.FontWatchRunner.LastResortFonts.SANS_SERIF)
};
webfont.FontWatchRunner.prototype.isLastResortFont_ = function(a, b) {
  return this.browserInfo_.hasWebKitFallbackBug() && this.widthsMatchLastResortWidths_(a, b)
};
webfont.FontWatchRunner.prototype.isMetricCompatibleFont_ = function() {
  return null === this.metricCompatibleFonts_ || this.metricCompatibleFonts_.hasOwnProperty(this.font_.getName())
};
webfont.FontWatchRunner.prototype.check_ = function() {
  var a = this.fontRulerA_.getWidth(), b = this.fontRulerB_.getWidth();
  this.isFallbackFont_(a, b) || this.isLastResortFont_(a, b) ? this.hasTimedOut_() ? this.isLastResortFont_(a, b) && this.isMetricCompatibleFont_() ? this.finish_(this.activeCallback_) : this.finish_(this.inactiveCallback_) : this.asyncCheck_() : this.finish_(this.activeCallback_)
};
webfont.FontWatchRunner.prototype.asyncCheck_ = function() {
  setTimeout(goog.bind(function() {
    this.check_()
  }, this), 25)
};
webfont.FontWatchRunner.prototype.finish_ = function(a) {
  this.fontRulerA_.remove();
  this.fontRulerB_.remove();
  a(this.font_)
};
// Input 13
webfont.FontWatcher = function(a, b, c, d) {
  this.domHelper_ = b;
  this.eventDispatcher_ = c;
  this.currentlyWatched_ = 0;
  this.success_ = this.last_ = !1;
  this.timeout_ = d;
  this.browserInfo_ = a.getBrowserInfo()
};
webfont.FontWatcher.prototype.watchFonts = function(a, b, c, d) {
  if(0 === a.length && d) {
    this.eventDispatcher_.dispatchInactive()
  }else {
    this.currentlyWatched_ += a.length;
    d && (this.last_ = d);
    for(d = 0;d < a.length;d++) {
      var e = a[d], f = b[e.getName()];
      this.eventDispatcher_.dispatchFontLoading(e);
      (new webfont.FontWatchRunner(goog.bind(this.fontActive_, this), goog.bind(this.fontInactive_, this), this.domHelper_, e, this.browserInfo_, this.timeout_, c, f)).start()
    }
  }
};
webfont.FontWatcher.prototype.fontActive_ = function(a) {
  this.eventDispatcher_.dispatchFontActive(a);
  this.success_ = !0;
  this.decreaseCurrentlyWatched_()
};
webfont.FontWatcher.prototype.fontInactive_ = function(a) {
  this.eventDispatcher_.dispatchFontInactive(a);
  this.decreaseCurrentlyWatched_()
};
webfont.FontWatcher.prototype.decreaseCurrentlyWatched_ = function() {
  0 == --this.currentlyWatched_ && this.last_ && (this.success_ ? this.eventDispatcher_.dispatchActive() : this.eventDispatcher_.dispatchInactive())
};
// Input 14
webfont.WebFont = function(a) {
  this.mainWindow_ = a;
  this.fontModuleLoader_ = new webfont.FontModuleLoader;
  this.userAgentParser_ = new webfont.UserAgentParser(a.navigator.userAgent, a.document);
  this.userAgent_ = this.userAgentParser_.parse();
  this.moduleFailedLoading_ = this.moduleLoading_ = 0
};
webfont.WebFont.prototype.addModule = function(a, b) {
  this.fontModuleLoader_.addModuleFactory(a, b)
};
webfont.WebFont.prototype.load = function(a) {
  var b = a.context || this.mainWindow_;
  this.domHelper_ = new webfont.DomHelper(this.mainWindow_, b);
  b = new webfont.EventDispatcher(this.domHelper_, b.document.documentElement, a);
  this.load_(b, a)
};
webfont.WebFont.prototype.isModuleSupportingUserAgent_ = function(a, b, c, d) {
  var e = this;
  d ? a.load(function(a, d, h) {
    e.onModuleReady_(b, c, a, d, h)
  }) : (a = 0 == --this.moduleLoading_, this.moduleFailedLoading_--, a && 0 == this.moduleFailedLoading_ ? b.dispatchInactive() : c.watchFonts([], {}, null, a))
};
webfont.WebFont.prototype.onModuleReady_ = function(a, b, c, d, e) {
  var f = 0 == --this.moduleLoading_;
  setTimeout(function() {
    b.watchFonts(c, d || {}, e || null, f)
  }, 0)
};
webfont.WebFont.prototype.load_ = function(a, b) {
  var c = [], d = b.timeout;
  a.dispatchLoading();
  c = this.fontModuleLoader_.getModules(b, this.domHelper_);
  this.moduleFailedLoading_ = this.moduleLoading_ = c.length;
  for(var d = new webfont.FontWatcher(this.userAgent_, this.domHelper_, a, d), e = 0, f = c.length;e < f;e++) {
    var g = c[e];
    g.supportUserAgent(this.userAgent_, goog.bind(this.isModuleSupportingUserAgent_, this, g, a, d))
  }
};
// Input 15
webfont.modules = {};
webfont.modules.google = {};
webfont.modules.google.FontApiUrlBuilder = function(a, b, c) {
  this.apiUrl_ = a ? a : b + webfont.modules.google.FontApiUrlBuilder.DEFAULT_API_URL;
  this.fontFamilies_ = [];
  this.subsets_ = [];
  this.text_ = c || ""
};
webfont.modules.google.FontApiUrlBuilder.DEFAULT_API_URL = "//fonts.googleapis.com/css";
webfont.modules.google.FontApiUrlBuilder.prototype.setFontFamilies = function(a) {
  this.parseFontFamilies_(a)
};
webfont.modules.google.FontApiUrlBuilder.prototype.parseFontFamilies_ = function(a) {
  for(var b = a.length, c = 0;c < b;c++) {
    var d = a[c].split(":");
    3 == d.length && this.subsets_.push(d.pop());
    var e = "";
    2 == d.length && "" != d[1] && (e = ":");
    this.fontFamilies_.push(d.join(e))
  }
};
webfont.modules.google.FontApiUrlBuilder.prototype.webSafe = function(a) {
  return a.replace(/ /g, "+")
};
webfont.modules.google.FontApiUrlBuilder.prototype.build = function() {
  if(0 == this.fontFamilies_.length) {
    throw Error("No fonts to load!");
  }
  if(-1 != this.apiUrl_.indexOf("kit=")) {
    return this.apiUrl_
  }
  for(var a = this.fontFamilies_.length, b = [], c = 0;c < a;c++) {
    b.push(this.webSafe(this.fontFamilies_[c]))
  }
  a = this.apiUrl_ + "?family=" + b.join("%7C");
  0 < this.subsets_.length && (a += "&subset=" + this.subsets_.join(","));
  0 < this.text_.length && (a += "&text=" + encodeURIComponent(this.text_));
  return a
};
// Input 16
webfont.modules.google.FontApiParser = function(a) {
  this.fontFamilies_ = a;
  this.parsedFonts_ = [];
  this.fontTestStrings_ = {}
};
webfont.modules.google.FontApiParser.INT_FONTS = {latin:webfont.FontWatchRunner.DEFAULT_TEST_STRING, cyrillic:"&#1081;&#1103;&#1046;", greek:"&#945;&#946;&#931;", khmer:"&#x1780;&#x1781;&#x1782;", Hanuman:"&#x1780;&#x1781;&#x1782;"};
webfont.modules.google.FontApiParser.WEIGHTS = {thin:"1", extralight:"2", "extra-light":"2", ultralight:"2", "ultra-light":"2", light:"3", regular:"4", book:"4", medium:"5", "semi-bold":"6", semibold:"6", "demi-bold":"6", demibold:"6", bold:"7", "extra-bold":"8", extrabold:"8", "ultra-bold":"8", ultrabold:"8", black:"9", heavy:"9", l:"3", r:"4", b:"7"};
webfont.modules.google.FontApiParser.STYLES = {i:"i", italic:"i", n:"n", normal:"n"};
webfont.modules.google.FontApiParser.VARIATION_MATCH = RegExp("^(thin|(?:(?:extra|ultra)-?)?light|regular|book|medium|(?:(?:semi|demi|extra|ultra)-?)?bold|black|heavy|l|r|b|[1-9]00)?(n|i|normal|italic)?$");
webfont.modules.google.FontApiParser.prototype.parse = function() {
  for(var a = this.fontFamilies_.length, b = 0;b < a;b++) {
    var c = this.fontFamilies_[b].split(":"), d = c[0].replace(/\+/g, " "), e = ["n4"];
    if(2 <= c.length) {
      var f = this.parseVariations_(c[1]);
      0 < f.length && (e = f);
      3 == c.length && (c = this.parseSubsets_(c[2]), 0 < c.length && (c = webfont.modules.google.FontApiParser.INT_FONTS[c[0]]) && (this.fontTestStrings_[d] = c))
    }
    this.fontTestStrings_[d] || (c = webfont.modules.google.FontApiParser.INT_FONTS[d]) && (this.fontTestStrings_[d] = c);
    for(c = 0;c < e.length;c += 1) {
      this.parsedFonts_.push(new webfont.Font(d, e[c]))
    }
  }
};
webfont.modules.google.FontApiParser.prototype.generateFontVariationDescription_ = function(a) {
  if(!a.match(/^[\w-]+$/)) {
    return""
  }
  a = a.toLowerCase();
  var b = webfont.modules.google.FontApiParser.VARIATION_MATCH.exec(a);
  if(null == b) {
    return""
  }
  a = this.normalizeStyle_(b[2]);
  b = this.normalizeWeight_(b[1]);
  return[a, b].join("")
};
webfont.modules.google.FontApiParser.prototype.normalizeStyle_ = function(a) {
  return null == a || "" == a ? "n" : webfont.modules.google.FontApiParser.STYLES[a]
};
webfont.modules.google.FontApiParser.prototype.normalizeWeight_ = function(a) {
  if(null == a || "" == a) {
    return"4"
  }
  var b = webfont.modules.google.FontApiParser.WEIGHTS[a];
  return b ? b : isNaN(a) ? "4" : a.substr(0, 1)
};
webfont.modules.google.FontApiParser.prototype.parseVariations_ = function(a) {
  var b = [];
  if(!a) {
    return b
  }
  a = a.split(",");
  for(var c = a.length, d = 0;d < c;d++) {
    var e = this.generateFontVariationDescription_(a[d]);
    e && b.push(e)
  }
  return b
};
webfont.modules.google.FontApiParser.prototype.parseSubsets_ = function(a) {
  var b = [];
  return!a ? b : a.split(",")
};
webfont.modules.google.FontApiParser.prototype.getFonts = function() {
  return this.parsedFonts_
};
webfont.modules.google.FontApiParser.prototype.getFontTestStrings = function() {
  return this.fontTestStrings_
};
// Input 17
webfont.modules.google.GoogleFontApi = function(a, b) {
  this.userAgent_ = (new webfont.UserAgentParser(navigator.userAgent, document)).parse();
  this.domHelper_ = a;
  this.configuration_ = b
};
webfont.modules.google.GoogleFontApi.NAME = "google";
webfont.modules.google.GoogleFontApi.METRICS_COMPATIBLE_FONTS = {Arimo:!0, Cousine:!0, Tinos:!0};
webfont.modules.google.GoogleFontApi.prototype.supportUserAgent = function(a, b) {
  b(a.getBrowserInfo().hasWebFontSupport())
};
webfont.modules.google.GoogleFontApi.prototype.load = function(a) {
  var b = this.domHelper_;
  "MSIE" == this.userAgent_.getName() && !0 != this.configuration_.blocking ? b.whenBodyExists(goog.bind(this.insertLink_, this, a)) : this.insertLink_(a)
};
webfont.modules.google.GoogleFontApi.prototype.insertLink_ = function(a) {
  var b = this.domHelper_, c = new webfont.modules.google.FontApiUrlBuilder(this.configuration_.api, b.getProtocol(), this.configuration_.text), d = this.configuration_.families;
  c.setFontFamilies(d);
  d = new webfont.modules.google.FontApiParser(d);
  d.parse();
  b.loadStylesheet(c.build());
  a(d.getFonts(), d.getFontTestStrings(), webfont.modules.google.GoogleFontApi.METRICS_COMPATIBLE_FONTS)
};
// Input 18
webfont.modules.Fontdeck = function(a, b) {
  this.domHelper_ = a;
  this.configuration_ = b;
  this.fonts_ = []
};
webfont.modules.Fontdeck.NAME = "fontdeck";
webfont.modules.Fontdeck.HOOK = "__webfontfontdeckmodule__";
webfont.modules.Fontdeck.API = "//f.fontdeck.com/s/css/js/";
webfont.modules.Fontdeck.prototype.getScriptSrc = function(a) {
  var b = this.domHelper_.getProtocol(), c = this.domHelper_.getHostName();
  return b + (this.configuration_.api || webfont.modules.Fontdeck.API) + c + "/" + a + ".js"
};
webfont.modules.Fontdeck.prototype.supportUserAgent = function(a, b) {
  var c = this.configuration_.id, d = this.domHelper_.getLoadWindow(), e = this;
  c ? (d[webfont.modules.Fontdeck.HOOK] || (d[webfont.modules.Fontdeck.HOOK] = {}), d[webfont.modules.Fontdeck.HOOK][c] = function(a, c) {
    for(var d = 0, j = c.fonts.length;d < j;++d) {
      var k = c.fonts[d];
      e.fonts_.push(new webfont.Font(k.name, webfont.Font.parseCssVariation("font-weight:" + k.weight + ";font-style:" + k.style)))
    }
    b(a)
  }, this.domHelper_.loadScript(this.getScriptSrc(c), function(a) {
    a && b(!1)
  })) : b(!1)
};
webfont.modules.Fontdeck.prototype.load = function(a) {
  a(this.fonts_)
};
// Input 19
webfont.modules.Typekit = function(a, b) {
  this.domHelper_ = a;
  this.configuration_ = b;
  this.fonts_ = []
};
webfont.modules.Typekit.NAME = "typekit";
webfont.modules.Typekit.HOOK = "__webfonttypekitmodule__";
webfont.modules.Typekit.prototype.getScriptSrc = function(a) {
  var b = this.domHelper_.getProtocol();
  return(this.configuration_.api || b + "//use.typekit.net") + "/" + a + ".js"
};
webfont.modules.Typekit.prototype.supportUserAgent = function(a, b) {
  var c = this.configuration_.id, d = this.configuration_, e = this.domHelper_.getLoadWindow(), f = this;
  c ? (e[webfont.modules.Typekit.HOOK] || (e[webfont.modules.Typekit.HOOK] = {}), e[webfont.modules.Typekit.HOOK][c] = function(c) {
    c(a, d, function(a, c, d) {
      for(var e = 0;e < c.length;e += 1) {
        var g = d[c[e]];
        if(g) {
          for(var l = 0;l < g.length;l += 1) {
            f.fonts_.push(new webfont.Font(c[e], g[l]))
          }
        }else {
          f.fonts_.push(new webfont.Font(c[e]))
        }
      }
      b(a)
    })
  }, this.domHelper_.loadScript(this.getScriptSrc(c), function(a) {
    a && b(!1)
  }, 2E3)) : b(!1)
};
webfont.modules.Typekit.prototype.load = function(a) {
  a(this.fonts_)
};
// Input 20
webfont.modules.Monotype = function(a, b) {
  this.domHelper_ = a;
  this.configuration_ = b;
  this.fonts_ = []
};
webfont.modules.Monotype.NAME = "monotype";
webfont.modules.Monotype.HOOK = "__mti_fntLst";
webfont.modules.Monotype.SCRIPTID = "__MonotypeAPIScript__";
webfont.modules.Monotype.prototype.supportUserAgent = function(a, b) {
  var c = this, d = c.configuration_.projectId, e = c.configuration_.version;
  if(d) {
    var f = c.domHelper_.getLoadWindow();
    this.domHelper_.loadScript(c.getScriptSrc(d, e), function(e) {
      if(e) {
        b(!1)
      }else {
        if(f[webfont.modules.Monotype.HOOK + d] && (e = f[webfont.modules.Monotype.HOOK + d]())) {
          for(var h = 0;h < e.length;h++) {
            c.fonts_.push(new webfont.Font(e[h].fontfamily))
          }
        }
        b(a.getBrowserInfo().hasWebFontSupport())
      }
    }).id = webfont.modules.Monotype.SCRIPTID + d
  }else {
    b(!1)
  }
};
webfont.modules.Monotype.prototype.getScriptSrc = function(a, b) {
  var c = this.domHelper_.getProtocol(), d = (this.configuration_.api || "fast.fonts.net/jsapi").replace(/^.*http(s?):(\/\/)?/, "");
  return c + "//" + d + "/" + a + ".js" + (b ? "?v=" + b : "")
};
webfont.modules.Monotype.prototype.load = function(a) {
  a(this.fonts_)
};
// Input 21
webfont.modules.Custom = function(a, b) {
  this.domHelper_ = a;
  this.configuration_ = b
};
webfont.modules.Custom.NAME = "custom";
webfont.modules.Custom.prototype.load = function(a) {
  var b, c, d = this.configuration_.urls || [], e = this.configuration_.families || [], f = this.configuration_.testStrings || {};
  b = 0;
  for(c = d.length;b < c;b++) {
    this.domHelper_.loadStylesheet(d[b])
  }
  d = [];
  b = 0;
  for(c = e.length;b < c;b++) {
    var g = e[b].split(":");
    if(g[1]) {
      for(var h = g[1].split(","), j = 0;j < h.length;j += 1) {
        d.push(new webfont.Font(g[0], h[j]))
      }
    }else {
      d.push(new webfont.Font(g[0]))
    }
  }
  a(d, f)
};
webfont.modules.Custom.prototype.supportUserAgent = function(a, b) {
  return b(a.getBrowserInfo().hasWebFontSupport())
};
// Input 22
var INCLUDE_CUSTOM_MODULE = !0, INCLUDE_FONTDECK_MODULE = !0, INCLUDE_MONOTYPE_MODULE = !0, INCLUDE_TYPEKIT_MODULE = !0, INCLUDE_GOOGLE_MODULE = !0, WEBFONT = "WebFont", WEBFONT_CONFIG = "WebFontConfig", webFontLoader = new webfont.WebFont(goog.global);
INCLUDE_CUSTOM_MODULE && webFontLoader.addModule(webfont.modules.Custom.NAME, function(a, b) {
  return new webfont.modules.Custom(b, a)
});
INCLUDE_FONTDECK_MODULE && webFontLoader.addModule(webfont.modules.Fontdeck.NAME, function(a, b) {
  return new webfont.modules.Fontdeck(b, a)
});
INCLUDE_MONOTYPE_MODULE && webFontLoader.addModule(webfont.modules.Monotype.NAME, function(a, b) {
  return new webfont.modules.Monotype(b, a)
});
INCLUDE_TYPEKIT_MODULE && webFontLoader.addModule(webfont.modules.Typekit.NAME, function(a, b) {
  return new webfont.modules.Typekit(b, a)
});
INCLUDE_GOOGLE_MODULE && webFontLoader.addModule(webfont.modules.google.GoogleFontApi.NAME, function(a, b) {
  return new webfont.modules.google.GoogleFontApi(b, a)
});
goog.global[WEBFONT] || (goog.global[WEBFONT] = {}, goog.global[WEBFONT].load = goog.bind(webFontLoader.load, webFontLoader), goog.global[WEBFONT_CONFIG] && webFontLoader.load(goog.global[WEBFONT_CONFIG]));
})(this,document);

