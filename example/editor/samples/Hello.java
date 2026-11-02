public class Hello {
    @Override
    public String toString() {
        // A text block can contain ordinary quote characters.
        String message = """
                { "name": "SourceView", "editable": true }
                """;
        return message;
    }

    public static void main(String[] args) {
        System.out.println(new Hello());
    }
}
