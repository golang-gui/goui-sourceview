<?php
declare(strict_types=1);
$name = 'SourceView';
$message = <<<TEXT
Hello, $name!
PHP interpolation keeps its own state.
TEXT;
$literal = <<<'TEXT'
$variables and "quotes" stay literal here.
TEXT;
?>
<!DOCTYPE html>
<html lang="en">
<body>
  <h1 title="<?= htmlspecialchars($name) ?>">Hello, <?= $name ?></h1>
  <pre><?= htmlspecialchars($message) ?></pre>
  <script>
    const name = "<?= $name ?>";
    console.log(name);
  </script>
</body>
</html>
