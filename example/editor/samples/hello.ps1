function Get-Greeting {
    param([string] $Name = 'SourceView')
    $message = @"
Hello, $Name!
PowerShell version: $($PSVersionTable.PSVersion)
"@
    $literal = @'
Quotes and $variables stay literal here.
'@
    if ($Name -ne '') {
        Write-Output $message
        Write-Output $literal
    }
}

<# Original PowerShell sample for SourceView. #>
Get-Greeting -Name 'SourceView'
