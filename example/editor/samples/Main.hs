-- Original Haskell sample for SourceView.
{-# LANGUAGE OverloadedStrings #-}
module Main where

{- Nested {- comments -} preserve their state. -}
greet :: String -> String
greet name = "Hello, " ++ name ++ "!"

main :: IO ()
main = do
  let languages = ["JSON", "XML", "HTML"]
  mapM_ (putStrLn . greet) languages
