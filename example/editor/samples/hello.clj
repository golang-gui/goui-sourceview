; Original Clojure sample for SourceView.
(ns editor.core)

#_(def ignored "This entire form is discarded.")

(defn greet [name]
  (let [theme :modern
        pattern #"[a-z_]+"
        message (str "Hello, " name "!")]
    {:message message
     :theme theme
     :valid (boolean (re-matches pattern name))}))
